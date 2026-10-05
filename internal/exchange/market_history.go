package exchange

import (
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"sort"
	"strings"
	"time"
)

type HistoryPoint struct {
	Time         int64 `json:"time"` // 实际观察时间，毫秒。
	ActionPoints int64 `json:"actionPoints"`
}
type HistoryReport struct {
	InstanceID string         `json:"instanceId"`
	PublicKey  string         `json:"publicKey"`
	Month      string         `json:"month"`
	IssuedAt   int64          `json:"issuedAt"`
	Points     []HistoryPoint `json:"points"`
	Count      int64          `json:"count"`
	Digest     string         `json:"digest"`
	Signature  string         `json:"signature"`
}
type historyWrite struct {
	ID          int64
	Report      HistoryReport
	Provisional bool // 仅 Mock 行情夹具；不计入玩家原始记录的校对摘要。
}
type HistoryManifest struct {
	Month        string `json:"month"`
	Count        int64  `json:"count"`
	Digest       string `json:"digest"`
	First        int64  `json:"firstObservedAt"`
	Last         int64  `json:"lastObservedAt"`
	ReconciledAt int64  `json:"reconciledAt"`
}

func monthRange(month string, now time.Time) (int64, int64, error) {
	l, _ := loadLocation("Asia/Shanghai")
	start, err := time.ParseInLocation("2006-01", month, l)
	current := time.Date(now.In(l).Year(), now.In(l).Month(), 1, 0, 0, 0, 0, l)
	if err != nil || start.After(current) || start.Before(current.AddDate(0, -12, 0)) {
		return 0, 0, fail("INVALID_MONTH", "历史月份须为本月或过去 12 个月")
	}
	return start.UnixMilli(), start.AddDate(0, 1, 0).UnixMilli(), nil
}
func historyCanonical(r HistoryReport) string {
	var text strings.Builder
	fmt.Fprintf(&text, "mmex-history-v1\n%s\n%s\n%s\n%d\n%d\n%s\n", r.InstanceID, r.PublicKey, r.Month, r.IssuedAt, r.Count, r.Digest)
	for _, p := range r.Points {
		fmt.Fprintf(&text, "%d:%d\n", p.Time, p.ActionPoints)
	}
	return text.String()
}
func verifyHistory(r HistoryReport, now time.Time) (*InstanceBinding, error) {
	if len(r.Points) == 0 && len(r.Digest) != 64 || r.Count < 0 {
		return nil, fail("INVALID_HISTORY", "历史批次或校对字段无效")
	}
	from, to, err := monthRange(r.Month, now)
	if err != nil {
		return nil, err
	}
	if !instancePattern.MatchString(r.InstanceID) || r.IssuedAt < now.Unix()-120 || r.IssuedAt > now.Unix()+60 {
		return nil, fail("INVALID_INSTANCE", "历史上传的实例身份或签发时间无效")
	}
	if r.Digest != "" {
		if _, err := hex.DecodeString(r.Digest); err != nil || len(r.Digest) != 64 {
			return nil, fail("INVALID_HISTORY", "历史摘要格式无效")
		}
	}
	previous := int64(-1)
	for _, p := range r.Points {
		if p.Time <= previous || p.Time < from || p.Time >= to || p.Time > now.UnixMilli()+60000 || p.ActionPoints < 0 || p.ActionPoints > 1_000_000 {
			return nil, fail("INVALID_HISTORY", "历史须按毫秒递增，时间和行动力需在允许范围")
		}
		previous = p.Time
	}
	pub, err := base64.StdEncoding.DecodeString(r.PublicKey)
	if err != nil || len(pub) != 32 {
		return nil, fail("INVALID_INSTANCE", "实例公钥无效")
	}
	sig, err := base64.StdEncoding.DecodeString(r.Signature)
	if err != nil || !ed25519.Verify(pub, []byte(historyCanonical(r)), sig) {
		return nil, fail("INVALID_INSTANCE", "历史上传签名无效")
	}
	key := sha256.Sum256([]byte(r.PublicKey + "\n" + r.InstanceID))
	return &InstanceBinding{hex.EncodeToString(key[:]), r.InstanceID, r.PublicKey}, nil
}
func (e *Engine) UploadOwner(token, key string) (int64, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	id := e.uploads[hashToken(token)]
	p := e.players[id]
	if p == nil || p.Disabled || p.Binding == nil || p.Binding.Key != key {
		return 0, fail("UNAUTHORIZED", "历史上传凭据或绑定实例无效")
	}
	return id, nil
}
func (e *Engine) IngestHistory(token string, r HistoryReport, binding *InstanceBinding) error {
	return e.transaction(func() error {
		id := e.uploads[hashToken(token)]
		p := e.players[id]
		if p == nil || p.Disabled || p.Binding == nil || binding == nil || p.Binding.Key != binding.Key {
			return fail("UNAUTHORIZED", "历史上传凭据或绑定实例无效")
		}
		now := e.clock()
		if err := e.advance(now); err != nil {
			return err
		}
		p = e.players[id]
		e.histories = append(e.histories, historyWrite{ID: id, Report: r})
		if len(r.Points) > 0 {
			last := r.Points[len(r.Points)-1]
			observed := last.Time / 1000
			var preciseLatest int64
			if err := e.db.QueryRow("SELECT COALESCE(MAX(time),0) FROM quote_history WHERE stock_id=? AND precise=1", id).Scan(&preciseLatest); err != nil {
				return err
			}
			// 补传仅修正行情，不重放历史交易；只处理仍然新鲜的最新记录。
			if last.Time >= preciseLatest && observed >= p.Quote.ObservedAt && observed >= now.Unix()-86400 && observed <= now.Unix()+60 && (observed > p.Quote.ObservedAt || last.ActionPoints*100 != p.Quote.Price) {
				for holder := range e.watchers[id] {
					e.accrue(e.touch(holder), now.Unix())
				}
				p = e.touch(id)
				p.Quote = Quote{Price: last.ActionPoints * 100, Previous: p.Quote.Price, ObservedAt: observed, UploadedAt: now.Unix()}
				e.delistIfNeeded(p, now)
				for holder := range e.watchers[id] {
					p := e.touch(holder)
					e.risk(p, now)
					e.processOrders(p, now)
				}
			}
		}
		return nil
	})
}

type historyQuery interface {
	Query(string, ...any) (*sql.Rows, error)
}

func writeHistoryHash(h hash.Hash, t, price int64) { fmt.Fprintf(h, "%d:%d\n", t, price/100) }
func historyManifest(q historyQuery, id int64, month string, now time.Time) (HistoryManifest, error) {
	from, to, err := monthRange(month, now)
	if err != nil {
		return HistoryManifest{}, err
	}
	out := HistoryManifest{Month: month}
	h := sha256.New()
	rows, err := q.Query("SELECT time,price FROM quote_history WHERE stock_id=? AND time>=? AND time<? AND precise=1 ORDER BY time", id, from, to)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var t, p int64
		if err = rows.Scan(&t, &p); err != nil {
			break
		}
		if out.Count == 0 {
			out.First = t
		}
		out.Last = t
		out.Count++
		writeHistoryHash(h, t, p)
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if rowErr != nil {
		return out, rowErr
	}
	out.Digest = hex.EncodeToString(h.Sum(nil))
	return out, nil
}
func (e *Engine) HistoryManifest(id int64, month string) (HistoryManifest, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	m, err := historyManifest(e.db, id, month, e.clock())
	if err != nil {
		return m, err
	}
	_ = e.db.QueryRow("SELECT checked_at FROM history_sync WHERE stock_id=? AND month=? AND digest=?", id, month, m.Digest).Scan(&m.ReconciledAt)
	return m, nil
}

func (e *Engine) migrateMarketData() error {
	var done int
	if err := e.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='market_data_migration'").Scan(&done); err != nil {
		return err
	}
	if done != 0 {
		return nil
	}
	tx, err := e.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT OR IGNORE INTO quote_history SELECT stock_id,time*1000,price,0 FROM quotes;
WITH ranked AS (
 SELECT *,time/60000*60000 AS bucket,
 ROW_NUMBER() OVER(PARTITION BY stock_id,time/60000 ORDER BY time) AS first,
 ROW_NUMBER() OVER(PARTITION BY stock_id,time/60000 ORDER BY time DESC) AS last FROM quote_history
) INSERT OR IGNORE INTO minute_prices SELECT stock_id,bucket,MAX(CASE WHEN first=1 THEN price END),MAX(price),MIN(price),MAX(CASE WHEN last=1 THEN price END),MIN(time),MAX(time),COUNT(*) FROM ranked GROUP BY stock_id,bucket;
INSERT OR IGNORE INTO trades SELECT json_extract(r.data,'$.id'),json_extract(r.data,'$.stockId'),p.username,json_extract(r.data,'$.side'),json_extract(r.data,'$.kind'),json_extract(r.data,'$.quantity'),json_extract(r.data,'$.price'),json_extract(r.data,'$.executedAt')*1000,CASE WHEN json_extract(r.data,'$.clientId') LIKE 'system_%' THEN 1 ELSE 0 END FROM receipts r JOIN players p ON p.id=r.player_id WHERE json_extract(r.data,'$.status')='filled';
CREATE TABLE market_data_migration (id INTEGER PRIMARY KEY);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (e *Engine) writeMarketData(tx *sql.Tx, changed map[int64]bool) error {
	dirty := map[int64]map[int64]bool{}
	invalid := map[int64]map[string]bool{}
	zone, _ := loadLocation("Asia/Shanghai")
	mark := func(id, t int64) {
		if dirty[id] == nil {
			dirty[id] = map[int64]bool{}
		}
		dirty[id][t/60000*60000] = true
		if invalid[id] == nil {
			invalid[id] = map[string]bool{}
		}
		invalid[id][time.UnixMilli(t).In(zone).Format("2006-01")] = true
		changed[id] = true
	}
	insert, err := tx.Prepare(`INSERT INTO quote_history(stock_id,time,price,precise) VALUES(?,?,?,?) ON CONFLICT(stock_id,time) DO UPDATE SET price=excluded.price,precise=MAX(quote_history.precise,excluded.precise) WHERE (quote_history.precise=0 OR excluded.precise=1) AND (quote_history.price!=excluded.price OR quote_history.precise!=excluded.precise)`)
	if err != nil {
		return err
	}
	defer insert.Close()
	for _, p := range e.points {
		if p.Point.Price == 0 {
			continue
		} // 缺少初始记录时的零报价不是行动力观测。
		t := p.Point.Time * 1000
		var precise int
		if err = tx.QueryRow("SELECT COUNT(*) FROM quote_history WHERE stock_id=? AND time>=? AND time<? AND precise=1", p.ID, t, t+1000).Scan(&precise); err != nil {
			return err
		}
		if precise > 0 {
			continue
		}
		result, err := insert.Exec(p.ID, t, p.Point.Price, 0)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n > 0 {
			mark(p.ID, t)
		}
	}
	for _, batch := range e.histories {
		for _, p := range batch.Report.Points {
			floor := p.Time / 1000 * 1000
			precision := 1
			if batch.Provisional {
				precision = 0
			}
			var removed int64
			if precision == 1 {
				result, err := tx.Exec("DELETE FROM quote_history WHERE stock_id=? AND time>=? AND time<? AND precise=0", batch.ID, floor, floor+1000)
				if err != nil {
					return err
				}
				removed, _ = result.RowsAffected()
			}
			if removed > 0 {
				mark(batch.ID, p.Time)
			}
			result, err := insert.Exec(batch.ID, p.Time, p.ActionPoints*100, precision)
			if err != nil {
				return err
			}
			n, _ := result.RowsAffected()
			if n > 0 {
				mark(batch.ID, p.Time)
			}
		}
	}
	for id, months := range invalid {
		for month := range months {
			if _, err = tx.Exec("DELETE FROM history_sync WHERE stock_id=? AND month=?", id, month); err != nil {
				return err
			}
		}
	}
	for _, batch := range e.histories {
		if batch.Report.Digest != "" {
			manifest, err := historyManifest(tx, batch.ID, batch.Report.Month, e.clock())
			if err != nil {
				return err
			}
			if manifest.Count != batch.Report.Count || manifest.Digest != batch.Report.Digest {
				return fail("HISTORY_MISMATCH", "月历史尚未对齐，客户端将自动补传修正")
			}
			if _, err = tx.Exec("INSERT INTO history_sync VALUES(?,?,?,?,?) ON CONFLICT(stock_id,month) DO UPDATE SET count=excluded.count,digest=excluded.digest,checked_at=excluded.checked_at", batch.ID, manifest.Month, manifest.Count, manifest.Digest, e.clock().Unix()); err != nil {
				return err
			}
			changed[batch.ID] = true
		}
	}
	// 只重建受影响的分钟，保留原始毫秒数据，日 K 和各分钟 K 复用这些聚合。
	for id, minutes := range dirty {
		keys := make([]int64, 0, len(minutes))
		for t := range minutes {
			keys = append(keys, t)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		for _, bucket := range keys {
			rows, err := tx.Query("SELECT time,price FROM quote_history WHERE stock_id=? AND time>=? AND time<? ORDER BY time", id, bucket, bucket+60000)
			if err != nil {
				return err
			}
			var o, h, l, c, first, last, count int64
			for rows.Next() {
				var t, p int64
				if err = rows.Scan(&t, &p); err != nil {
					break
				}
				if count == 0 {
					o = p
					h = p
					l = p
					first = t
				}
				h = max(h, p)
				l = min(l, p)
				c = p
				last = t
				count++
			}
			rowErr := rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			if rowErr != nil {
				return rowErr
			}
			_, err = tx.Exec("INSERT INTO minute_prices VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(stock_id,time) DO UPDATE SET open=excluded.open,high=excluded.high,low=excluded.low,close=excluded.close,first_time=excluded.first_time,last_time=excluded.last_time,samples=excluded.samples", id, bucket, o, h, l, c, first, last, count)
			if err != nil {
				return err
			}
		}
	}
	for _, t := range e.trades {
		_, err := tx.Exec("INSERT INTO trades VALUES(?,?,?,?,?,?,?,?,?)", t.ID, t.StockID, t.Username, t.Side, t.Kind, t.Quantity, t.Price, t.Time, t.Forced)
		if err != nil {
			return err
		}
		changed[t.StockID] = true
	}
	return nil
}
