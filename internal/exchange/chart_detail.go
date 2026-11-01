package exchange

import (
	"net/http"
	"sort"
	"strconv"
	"time"
)

type DetailBar struct {
	Time       int64 `json:"time"`
	Open       int64 `json:"open"`
	High       int64 `json:"high"`
	Low        int64 `json:"low"`
	Close      int64 `json:"close"`
	Samples    int64 `json:"samples"`
	Volume     int64 `json:"volume"`
	Turnover   int64 `json:"turnover"`
	BuyVolume  int64 `json:"buyVolume"`
	SellVolume int64 `json:"sellVolume"`
}
type dayOpenReader interface {
	DayOpen(int64, int64, int64) (int64, int64, error)
}

func (m *mutation) updateOpening(s *Stock, points []Point) error {
	day := shanghaiDay(m.e.clock().UnixMilli())
	start, _ := time.ParseInLocation("2006-01-02", day, time.FixedZone("Asia/Shanghai", 8*3600))
	from := start.UnixMilli()
	to := from + 86400000 - 1
	at, price := s.OpenAt, s.Open
	if s.OpenDay != day {
		at, price = 0, 0
	}
	if reader, ok := m.e.store.(dayOpenReader); ok {
		t, p, err := reader.DayOpen(s.ID, from, to)
		if err != nil {
			return err
		}
		at, price = t, p
	}
	for _, p := range points {
		if p.T >= from && p.T <= to && (at == 0 || p.T <= at) {
			at, price = p.T, p.P*100
		}
	}
	s.OpenAt, s.Open, s.OpenDay = at, price, day
	return nil
}
func (s *PostgresStore) DayOpen(id, from, to int64) (int64, int64, error) {
	rows, err := s.db.Query("SELECT time_ms,action_points*100 FROM mmex_v2_history WHERE stock_id=$1 AND time_ms>=$2 AND time_ms<=$3 ORDER BY time_ms LIMIT 1", id, from, to)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	var t, p int64
	if rows.Next() {
		err = rows.Scan(&t, &p)
	}
	if err != nil {
		return 0, 0, err
	}
	return t, p, rows.Err()
}
func (e *Engine) DetailBars(id, from, to, interval int64) ([]DetailBar, error) {
	quotes, err := e.cachedChartEntry(id, from, to, interval)
	if err != nil {
		return nil, err
	}
	buckets := map[int64]*DetailBar{}
	times := map[int64][2]int64{}
	for key, value := range quotes.Times {
		times[key] = value
	}
	for _, c := range quotes.Points {
		buckets[c.T] = &DetailBar{Time: c.T, Open: c.O, High: c.H, Low: c.L, Close: c.C, Samples: c.N}
	}
	step := interval * 60000
	offset := int64(0)
	if step >= 86400000 {
		offset = 28800000
	}
	e.mu.RLock()
	for _, tid := range e.stockTrades[id] {
		t := e.trades[tid]
		if t.At < from || t.At > to {
			continue
		}
		bucket := (t.At+offset)/step*step - offset
		b := buckets[bucket]
		if b == nil {
			b = &DetailBar{Time: bucket, Open: t.Price, High: t.Price, Low: t.Price, Close: t.Price}
			buckets[bucket] = b
			times[bucket] = [2]int64{t.At, t.At}
		}
		b.High = max(b.High, t.Price)
		b.Low = min(b.Low, t.Price)
		bounds := times[bucket]
		if t.At < bounds[0] {
			b.Open = t.Price
			bounds[0] = t.At
		}
		if t.At >= bounds[1] {
			b.Close = t.Price
			bounds[1] = t.At
		}
		times[bucket] = bounds
		b.Volume += t.Quantity
		b.Turnover += t.Notional
		if buying(t.Side) {
			b.BuyVolume += t.Quantity
		} else {
			b.SellVolume += t.Quantity
		}
	}
	e.mu.RUnlock()
	out := make([]DetailBar, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out, nil
}
func (s *Server) chartDetail(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	now := s.engine.clock()
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	month := r.URL.Query().Get("month")
	if month == "" {
		month = now.In(loc).Format("2006-01")
	}
	start, err := time.ParseInLocation("2006-01", month, loc)
	if err != nil {
		s.result(w, r, nil, fail("INVALID_RANGE", "月份格式须为 YYYY-MM"))
		return
	}
	end := start.AddDate(0, 1, 0)
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "time"
	}
	interval := int64(1)
	display := start
	from := start.AddDate(0, 0, -2)
	day := r.URL.Query().Get("day")
	switch period {
	case "day":
		interval = 1440
		from = start.AddDate(0, -3, 0)
	case "time":
		if day == "" {
			points, _, err := s.engine.Chart(id, start.UnixMilli(), end.UnixMilli()-1, 1440)
			if err != nil {
				s.result(w, r, nil, err)
				return
			}
			if len(points) > 0 {
				day = shanghaiDay(points[len(points)-1].T)
			} else {
				day = start.Format("2006-01-02")
			}
		}
		date, err := time.ParseInLocation("2006-01-02", day, loc)
		if err != nil || date.Before(start) || !date.Before(end) {
			s.result(w, r, nil, fail("INVALID_RANGE", "分时日期须在所选月份内"))
			return
		}
		display = date
		from = date
		end = date.AddDate(0, 0, 1)
	case "m5":
		interval = 5
	case "m10":
		interval = 10
	case "m20":
		interval = 20
	case "m30":
		interval = 30
	case "m60":
		interval = 60
	default:
		s.result(w, r, nil, fail("INVALID_RANGE", "图表周期无效"))
		return
	}
	bars, err := s.engine.DetailBars(id, from.UnixMilli(), end.UnixMilli()-1, interval)
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	data, err := s.engine.StockData(id, start.UnixMilli(), start.AddDate(0, 1, 0).UnixMilli()-1)
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	months, err := s.engine.store.Manifest(id)
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	count := int64(0)
	for _, m := range months {
		if m.Month == month {
			count = m.Count
		}
	}
	summary := map[string]any{"day": day, "open": int64(0), "high": int64(0), "low": int64(0), "volume": int64(0), "turnover": int64(0), "buyVolume": int64(0), "sellVolume": int64(0), "previousClose": int64(0)}
	if period == "time" {
		if reader, ok := s.engine.store.(interface {
			PreviousQuote(int64, int64) (int64, error)
		}); ok {
			previous, err := reader.PreviousQuote(id, display.UnixMilli())
			if err != nil {
				s.result(w, r, nil, err)
				return
			}
			summary["previousClose"] = previous
		}
	}
	for _, b := range bars {
		if b.Time < display.UnixMilli() {
			summary["previousClose"] = b.Close
			continue
		}
		if summary["open"].(int64) == 0 {
			summary["open"] = b.Open
			summary["low"] = b.Low
		}
		summary["high"] = max(summary["high"].(int64), b.High)
		summary["low"] = min(summary["low"].(int64), b.Low)
		for key, v := range map[string]int64{"volume": b.Volume, "turnover": b.Turnover, "buyVolume": b.BuyVolume, "sellVolume": b.SellVolume} {
			summary[key] = summary[key].(int64) + v
		}
	}
	data["bars"], data["period"], data["month"], data["day"], data["displayFrom"], data["summary"], data["coverage"] = bars, period, month, day, display.UnixMilli(), summary, map[string]any{"count": count, "reconciledAt": data["stock"].(map[string]any)["historyCheckedAt"]}
	if reader, ok := s.engine.store.(interface {
		HistoryBounds(int64, int64, int64) (int64, int64, error)
	}); ok {
		first, last, err := reader.HistoryBounds(id, start.UnixMilli(), start.AddDate(0, 1, 0).UnixMilli()-1)
		if err != nil {
			s.result(w, r, nil, err)
			return
		}
		coverage := data["coverage"].(map[string]any)
		coverage["firstObservedAt"], coverage["lastObservedAt"] = first, last
	}
	s.json(w, r, 200, data)
}

func (s *PostgresStore) HistoryBounds(id, from, to int64) (int64, int64, error) {
	var first, last int64
	err := s.db.QueryRow("SELECT COALESCE(min(time_ms),0),COALESCE(max(time_ms),0) FROM mmex_v2_history WHERE stock_id=$1 AND time_ms>=$2 AND time_ms<=$3", id, from, to).Scan(&first, &last)
	return first, last, err
}

func (s *PostgresStore) PreviousQuote(id, before int64) (int64, error) {
	var price int64
	rows, err := s.db.Query("SELECT action_points*100 FROM mmex_v2_history WHERE stock_id=$1 AND time_ms<$2 ORDER BY time_ms DESC LIMIT 1", id, before)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if rows.Next() {
		err = rows.Scan(&price)
	}
	if err != nil {
		return 0, err
	}
	return price, rows.Err()
}
