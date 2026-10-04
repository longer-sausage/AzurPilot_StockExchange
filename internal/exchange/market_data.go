package exchange

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 成交与账本在同一事务落盘。行情公开接口仅暴露这些公开字段。
type Trade struct {
	ID       int64  `json:"id"`
	StockID  int64  `json:"stockId"`
	Username string `json:"username"`
	Side     string `json:"side"`
	Kind     string `json:"kind"`
	Quantity int64  `json:"quantity"`
	Price    int64  `json:"price"`
	Time     int64  `json:"time"`
	Forced   bool   `json:"forced"`
}
type Candle struct {
	Time       int64   `json:"time"`
	Open       int64   `json:"open"`
	High       int64   `json:"high"`
	Low        int64   `json:"low"`
	Close      int64   `json:"close"`
	Samples    int64   `json:"samples"`
	Volume     float64 `json:"volume"`
	Turnover   float64 `json:"turnover"`
	BuyVolume  float64 `json:"buyVolume"`
	SellVolume float64 `json:"sellVolume"`
}
type PublicPending struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Side      string `json:"side"`
	Quantity  int64  `json:"quantity"`
	Limit     int64  `json:"limit"`
	CreatedAt int64  `json:"createdAt"`
	TIF       string `json:"tif"`
	Kind      string `json:"kind"`
}
type HistoryCoverage struct {
	Count        int64 `json:"count"`
	First        int64 `json:"firstObservedAt"`
	Last         int64 `json:"lastObservedAt"`
	ReconciledAt int64 `json:"reconciledAt"`
}
type StockSummary struct {
	Day           string  `json:"day"`
	Open          int64   `json:"open"`
	High          int64   `json:"high"`
	Low           int64   `json:"low"`
	PreviousClose int64   `json:"previousClose"`
	Volume        float64 `json:"volume"`
	Turnover      float64 `json:"turnover"`
	BuyVolume     float64 `json:"buyVolume"`
	SellVolume    float64 `json:"sellVolume"`
}
type StockDetail struct {
	Stock       Stock           `json:"stock"`
	Period      string          `json:"period"`
	Month       string          `json:"month"`
	Day         string          `json:"day"`
	DisplayFrom int64           `json:"displayFrom"`
	Bars        []Candle        `json:"bars"`
	Trades      []Trade         `json:"trades"`
	Pending     []PublicPending `json:"pending"`
	Summary     StockSummary    `json:"summary"`
	Coverage    HistoryCoverage `json:"coverage"`
}
type detailCacheEntry struct {
	Data    []byte
	ETag    string
	Version uint64
	Until   int64
	Used    int64
}

const detailCacheLimit = 8 * 1024 * 1024

// 当日第一条行动力报价作为开盘价，迟到补传或修正后直接读取最新聚合。
func (e *Engine) openingPrices(now time.Time, id int64) (map[int64]int64, error) {
	zone, _ := loadLocation("Asia/Shanghai")
	day := now.In(zone)
	from := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, zone)
	query := `SELECT m.stock_id,m.open FROM minute_prices m JOIN
 (SELECT stock_id,MIN(time) AS first FROM minute_prices WHERE time>=? AND time<?`
	args := []any{from.UnixMilli(), from.AddDate(0, 0, 1).UnixMilli()}
	if id > 0 {
		query += " AND stock_id=?"
		args = append(args, id)
	}
	query += ` GROUP BY stock_id) d ON m.stock_id=d.stock_id AND m.time=d.first`
	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prices := map[int64]int64{}
	for rows.Next() {
		var stock, price int64
		if err := rows.Scan(&stock, &price); err != nil {
			return nil, err
		}
		prices[stock] = price
	}
	return prices, rows.Err()
}

func (e *Engine) marketStock(p *Player, open int64, now time.Time) Stock {
	return Stock{ID: p.ID, Symbol: symbol(p.ID), Username: p.Username, Quote: p.Quote, Stale: !e.quoteFresh(p, now.Unix()), Disabled: p.Disabled, Delisted: p.Delisted, Open: open}
}

func periodBucket(period string) (string, error) {
	if period == "day" {
		return "(time+28800000)/86400000*86400000-28800000", nil
	}
	minutes := map[string]int64{"time": 1, "m5": 5, "m10": 10, "m20": 20, "m30": 30, "m60": 60}[period]
	if minutes == 0 {
		return "", fail("INVALID_PERIOD", "支持分时、日 K、M5、M10、M20、M30、M60")
	}
	span := minutes * 60000
	return fmt.Sprintf("time/%d*%d", span, span), nil
}

// SQLite 从分钟表直接聚合，原始毫秒历史无需在每次看图时扫描。
func (e *Engine) candles(id, from, to int64, period string) ([]Candle, error) {
	bucket, err := periodBucket(period)
	if err != nil {
		return nil, err
	}
	query := `WITH grouped AS (SELECT ` + bucket + ` AS bucket,MIN(time) AS first,MAX(time) AS last,MAX(high) AS high,MIN(low) AS low,SUM(samples) AS samples FROM minute_prices WHERE stock_id=? AND time>=? AND time<? GROUP BY bucket)
SELECT g.bucket,a.open,g.high,g.low,z.close,g.samples FROM grouped g JOIN minute_prices a ON a.stock_id=? AND a.time=g.first JOIN minute_prices z ON z.stock_id=? AND z.time=g.last ORDER BY g.bucket`
	rows, err := e.db.Query(query, id, from, to, id, id)
	if err != nil {
		return nil, err
	}
	bars := []Candle{}
	index := map[int64]int{}
	for rows.Next() {
		var b Candle
		if err = rows.Scan(&b.Time, &b.Open, &b.High, &b.Low, &b.Close, &b.Samples); err != nil {
			break
		}
		index[b.Time] = len(bars)
		bars = append(bars, b)
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if rowErr != nil {
		return nil, rowErr
	}
	// 全部真实成交都保留；交易量按同一个时间桶汇总，含强制平仓。
	rows, err = e.db.Query(`SELECT `+bucket+`,SUM(CAST(quantity AS REAL)),SUM(CAST(quantity AS REAL)*price),SUM(CASE WHEN side IN ('buy','cover') THEN CAST(quantity AS REAL) ELSE 0 END),SUM(CASE WHEN side IN ('sell','short') THEN CAST(quantity AS REAL) ELSE 0 END),MIN(price),MAX(price) FROM trades WHERE stock_id=? AND time>=? AND time<? GROUP BY 1 ORDER BY 1`, id, from, to)
	if err != nil {
		return nil, err
	}
	type totals struct {
		t, low, high              int64
		volume, amount, buy, sell float64
	}
	aggregates := []totals{}
	for rows.Next() {
		var a totals
		if err = rows.Scan(&a.t, &a.volume, &a.amount, &a.buy, &a.sell, &a.low, &a.high); err != nil {
			break
		}
		aggregates = append(aggregates, a)
	}
	rowErr = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if rowErr != nil {
		return nil, rowErr
	}
	for _, a := range aggregates {
		i, ok := index[a.t]
		if !ok {
			// 没有新行动力观测但有成交的分钟，用实际成交价作平价 K 线。
			var open, close int64
			if err = e.dbTradePrices(id, a.t, period, &open, &close); err != nil {
				return nil, err
			}
			i = len(bars)
			index[a.t] = i
			bars = append(bars, Candle{Time: a.t, Open: open, High: a.high, Low: a.low, Close: close})
		}
		bars[i].Volume = a.volume
		bars[i].Turnover = a.amount
		bars[i].BuyVolume = a.buy
		bars[i].SellVolume = a.sell
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Time < bars[j].Time })
	return bars, nil
}

func (e *Engine) dbTradePrices(id, t int64, period string, open, close *int64) error {
	// 此函数由 candles 收集完查询结果后调用，避免单连接数据库的嵌套查询。
	span := int64(86400000)
	if period != "day" {
		minutes := map[string]int64{"time": 1, "m5": 5, "m10": 10, "m20": 20, "m30": 30, "m60": 60}[period]
		span = minutes * 60000
	}
	if err := e.db.QueryRow("SELECT price FROM trades WHERE stock_id=? AND time>=? AND time<? ORDER BY time,id LIMIT 1", id, t, t+span).Scan(open); err != nil {
		return err
	}
	return e.db.QueryRow("SELECT price FROM trades WHERE stock_id=? AND time>=? AND time<? ORDER BY time DESC,id DESC LIMIT 1", id, t, t+span).Scan(close)
}

func (e *Engine) StockDetailJSON(id int64, period, month, day string) ([]byte, string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.clock()
	l, _ := loadLocation("Asia/Shanghai")
	p := e.players[id]
	if p == nil {
		return nil, "", fail("STOCK_NOT_FOUND", "证券不存在")
	}
	if period == "" {
		period = "time"
	}
	if _, err := periodBucket(period); err != nil {
		return nil, "", err
	}
	if month == "" {
		month = now.In(l).Format("2006-01")
	}
	from, to, err := monthRange(month, now)
	if err != nil {
		return nil, "", err
	}
	currentDay := now.In(l).Format("2006-01-02")
	key := fmt.Sprintf("%d/%s/%s/%s/%s/%d", id, period, month, day, currentDay, e.state.Settings.Active.QuoteTTLSeconds)
	if entry, ok := e.detailCache[key]; ok && entry.Version == e.stockVersions[id] && now.Unix() < entry.Until {
		entry.Used = now.Unix()
		e.detailCache[key] = entry
		return entry.Data, entry.ETag, nil
	}
	var latest int64
	if err = e.db.QueryRow("SELECT COALESCE(MAX(time),0) FROM minute_prices WHERE stock_id=? AND time>=? AND time<?", id, from, to).Scan(&latest); err != nil {
		return nil, "", err
	}
	if day == "" {
		if latest != 0 {
			day = time.UnixMilli(latest).In(l).Format("2006-01-02")
		} else {
			day = time.UnixMilli(from).In(l).Format("2006-01-02")
			if month == now.In(l).Format("2006-01") {
				day = currentDay
			}
		}
	}
	dayStart, err := time.ParseInLocation("2006-01-02", day, l)
	if err != nil || dayStart.UnixMilli() < from || dayStart.UnixMilli() >= to {
		return nil, "", fail("INVALID_DAY", "分时日期须在所选月份内")
	}
	chartFrom, chartTo, displayFrom := from, to, from
	if period == "time" {
		chartFrom = dayStart.UnixMilli()
		chartTo = dayStart.AddDate(0, 0, 1).UnixMilli()
		displayFrom = chartFrom
	}
	if period == "day" {
		chartFrom = time.UnixMilli(from).In(l).AddDate(0, -3, 0).UnixMilli()
	}
	if strings.HasPrefix(period, "m") {
		chartFrom = from - 2*86400000
	}
	opening, err := e.openingPrices(now, id)
	if err != nil {
		return nil, "", err
	}
	stock := e.marketStock(p, opening[id], now)
	out := StockDetail{Stock: stock, Period: period, Month: month, Day: day, DisplayFrom: displayFrom, Trades: []Trade{}, Pending: []PublicPending{}}
	out.Bars, err = e.candles(id, chartFrom, chartTo, period)
	if err != nil {
		return nil, "", err
	}
	daily, err := e.candles(id, dayStart.UnixMilli()-7*86400000, dayStart.AddDate(0, 0, 1).UnixMilli(), "day")
	if err != nil {
		return nil, "", err
	}
	out.Summary.Day = day
	for _, b := range daily {
		if b.Time < dayStart.UnixMilli() {
			out.Summary.PreviousClose = b.Close
		} else {
			out.Summary.Open = b.Open
			out.Summary.High = b.High
			out.Summary.Low = b.Low
			out.Summary.Volume = b.Volume
			out.Summary.Turnover = b.Turnover
			out.Summary.BuyVolume = b.BuyVolume
			out.Summary.SellVolume = b.SellVolume
		}
	}
	if out.Summary.PreviousClose == 0 {
		_ = e.db.QueryRow("SELECT close FROM minute_prices WHERE stock_id=? AND time<? ORDER BY time DESC LIMIT 1", id, dayStart.UnixMilli()).Scan(&out.Summary.PreviousClose)
	}
	rows, err := e.db.Query("SELECT id,stock_id,username,side,kind,quantity,price,time,forced FROM trades WHERE stock_id=? AND time>=? AND time<? ORDER BY time DESC,id DESC LIMIT 100", id, from, to)
	if err != nil {
		return nil, "", err
	}
	for rows.Next() {
		var t Trade
		if err = rows.Scan(&t.ID, &t.StockID, &t.Username, &t.Side, &t.Kind, &t.Quantity, &t.Price, &t.Time, &t.Forced); err != nil {
			break
		}
		out.Trades = append(out.Trades, t)
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil {
		return nil, "", err
	}
	if rowErr != nil {
		return nil, "", rowErr
	}
	for holder := range e.watchers[id] {
		p := e.players[holder]
		for _, o := range p.Orders {
			if o.StockID == id && o.Status == "pending" {
				out.Pending = append(out.Pending, PublicPending{o.ID, p.Username, o.Side, o.Quantity, o.Limit, o.CreatedAt, o.TIF, o.Kind})
			}
		}
	}
	sort.Slice(out.Pending, func(i, j int) bool { return out.Pending[i].ID > out.Pending[j].ID })
	if len(out.Pending) > 50 {
		out.Pending = out.Pending[:50]
	}
	if err = e.db.QueryRow("SELECT COUNT(*),COALESCE(MIN(time),0),COALESCE(MAX(time),0) FROM quote_history WHERE stock_id=? AND time>=? AND time<? AND precise=1", id, from, to).Scan(&out.Coverage.Count, &out.Coverage.First, &out.Coverage.Last); err != nil {
		return nil, "", err
	}
	_ = e.db.QueryRow("SELECT checked_at FROM history_sync WHERE stock_id=? AND month=?", id, month).Scan(&out.Coverage.ReconciledAt)
	data, err := json.Marshal(out)
	if err != nil {
		return nil, "", err
	}
	hash := sha256.Sum256(data)
	etag := fmt.Sprintf(`"stock-%x"`, hash[:12])
	until := now.Unix() + 60
	deadline := p.Quote.ObservedAt + e.state.Settings.Active.QuoteTTLSeconds + 1
	if deadline > now.Unix() {
		until = min(until, deadline)
	}
	if old, ok := e.detailCache[key]; ok {
		e.detailBytes -= len(old.Data)
		delete(e.detailCache, key)
	}
	for len(e.detailCache) > 0 && (len(e.detailCache) >= 24 || e.detailBytes+len(data) > detailCacheLimit) {
		var oldest string
		var used int64 = 1<<63 - 1
		for k, c := range e.detailCache {
			if c.Used < used {
				oldest = k
				used = c.Used
			}
		}
		e.detailBytes -= len(e.detailCache[oldest].Data)
		delete(e.detailCache, oldest)
	}
	if len(data) <= detailCacheLimit {
		e.detailCache[key] = detailCacheEntry{data, etag, e.stockVersions[id], until, now.Unix()}
		e.detailBytes += len(data)
	}
	return data, etag, nil
}

func (s *Server) stockDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("stock"), 10, 64)
	if err != nil || id <= 0 {
		s.error(w, 400, fail("INVALID_ID", "股票标识无效"))
		return
	}
	q := r.URL.Query()
	for k, values := range q {
		if k != "period" && k != "month" && k != "day" || len(values) != 1 {
			s.error(w, 400, fail("INVALID_QUERY", "行情查询参数无效"))
			return
		}
	}
	b, etag, err := s.engine.StockDetailJSON(id, q.Get("period"), q.Get("month"), q.Get("day"))
	if err != nil {
		s.error(w, 400, err)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(304)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}
func (s *Server) uploadHistory(w http.ResponseWriter, r *http.Request) {
	if !s.limit(r, "history", 30, 60) {
		s.error(w, 429, fail("RATE_LIMIT", "历史补传过于频繁，请稍后重试"))
		return
	}
	var in struct {
		Report HistoryReport `json:"report"`
	}
	if err := decode(w, r, &in); err != nil {
		s.error(w, 400, err)
		return
	}
	binding, err := verifyHistory(in.Report, s.engine.clock())
	if err != nil {
		s.error(w, 400, err)
		return
	}
	if r.Header.Get("X-MMEX-Instance") != binding.Key {
		s.error(w, 401, fail("UNAUTHORIZED", "绑定实例不匹配"))
		return
	}
	if err = s.engine.IngestHistory(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), in.Report, binding); err != nil {
		s.error(w, 400, err)
		return
	}
	s.json(w, 200, map[string]any{"ok": true, "accepted": len(in.Report.Points), "month": in.Report.Month})
}
func (s *Server) historyManifest(w http.ResponseWriter, r *http.Request) {
	if !s.limit(r, "manifest", 12, 60) {
		s.error(w, 429, fail("RATE_LIMIT", "月历史校对过于频繁"))
		return
	}
	id, err := s.engine.UploadOwner(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), r.Header.Get("X-MMEX-Instance"))
	if err != nil {
		s.error(w, 401, err)
		return
	}
	m, err := s.engine.HistoryManifest(id, r.URL.Query().Get("month"))
	if err != nil {
		s.error(w, 400, err)
		return
	}
	s.json(w, 200, m)
}
