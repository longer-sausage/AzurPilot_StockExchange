package exchange

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	_ "github.com/lib/pq"
	"strconv"
	"time"
)

// PostgresStore 按实体写入受影响行；完整历史独立存储，不随账本反复序列化。
type PostgresStore struct {
	db   *sql.DB
	lock *sql.Conn
}

func OpenPostgres(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("Postgres 连接失败")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}
	var locked bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(718890246235)").Scan(&locked); err != nil || !locked {
		conn.Close()
		db.Close()
		return nil, fmt.Errorf("同一数据库只允许一个交易所进程运行")
	}
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS mmex_v2_entities (kind TEXT NOT NULL,id TEXT NOT NULL,data JSONB NOT NULL,PRIMARY KEY(kind,id));
 CREATE TABLE IF NOT EXISTS mmex_v2_history (stock_id BIGINT NOT NULL,time_ms BIGINT NOT NULL,action_points BIGINT NOT NULL CHECK(action_points>=0),PRIMARY KEY(stock_id,time_ms));
 CREATE TABLE IF NOT EXISTS mmex_v2_months (stock_id BIGINT NOT NULL,month TEXT NOT NULL,count BIGINT NOT NULL DEFAULT 0,digest TEXT NOT NULL DEFAULT '',dirty BOOLEAN NOT NULL DEFAULT true,PRIMARY KEY(stock_id,month));
 DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM mmex_v2_entities WHERE kind='schema' AND id='history-months-v2-shanghai') THEN
 DELETE FROM mmex_v2_months;
 INSERT INTO mmex_v2_months(stock_id,month) SELECT DISTINCT stock_id,to_char(to_timestamp(time_ms/1000) AT TIME ZONE 'Asia/Shanghai','YYYY-MM') FROM mmex_v2_history ON CONFLICT DO NOTHING;
 INSERT INTO mmex_v2_entities(kind,id,data) VALUES('schema','history-months-v2-shanghai','{}') ON CONFLICT DO NOTHING;
 END IF; END $$;`)
	if err == nil {
		_, err = db.ExecContext(ctx, `
 CREATE OR REPLACE FUNCTION mmex_v2_history_dirty_new() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 INSERT INTO mmex_v2_months(stock_id,month) SELECT DISTINCT stock_id,to_char(to_timestamp(time_ms/1000) AT TIME ZONE 'Asia/Shanghai','YYYY-MM') FROM new_rows ON CONFLICT(stock_id,month) DO UPDATE SET dirty=true; RETURN NULL; END $$;
 CREATE OR REPLACE FUNCTION mmex_v2_history_dirty_old() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 INSERT INTO mmex_v2_months(stock_id,month) SELECT DISTINCT stock_id,to_char(to_timestamp(time_ms/1000) AT TIME ZONE 'Asia/Shanghai','YYYY-MM') FROM old_rows ON CONFLICT(stock_id,month) DO UPDATE SET dirty=true; RETURN NULL; END $$;
 CREATE OR REPLACE FUNCTION mmex_v2_history_dirty_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 INSERT INTO mmex_v2_months(stock_id,month) SELECT DISTINCT stock_id,to_char(to_timestamp(time_ms/1000) AT TIME ZONE 'Asia/Shanghai','YYYY-MM') FROM (SELECT stock_id,time_ms FROM new_rows UNION SELECT stock_id,time_ms FROM old_rows) AS changed ON CONFLICT(stock_id,month) DO UPDATE SET dirty=true; RETURN NULL; END $$;
 CREATE OR REPLACE FUNCTION mmex_v2_history_dirty_truncate() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN UPDATE mmex_v2_months SET dirty=true; RETURN NULL; END $$;
 DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='mmex_v2_history_insert') THEN
 CREATE TRIGGER mmex_v2_history_insert AFTER INSERT ON mmex_v2_history REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION mmex_v2_history_dirty_new();
 CREATE TRIGGER mmex_v2_history_delete AFTER DELETE ON mmex_v2_history REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION mmex_v2_history_dirty_old();
 CREATE TRIGGER mmex_v2_history_update AFTER UPDATE ON mmex_v2_history REFERENCING OLD TABLE AS old_rows NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION mmex_v2_history_dirty_update();
 CREATE TRIGGER mmex_v2_history_truncate AFTER TRUNCATE ON mmex_v2_history FOR EACH STATEMENT EXECUTE FUNCTION mmex_v2_history_dirty_truncate();
 END IF; END $$;`)
	}
	if err != nil {
		conn.Close()
		db.Close()
		return nil, err
	}
	return &PostgresStore{db, conn}, nil
}
func (s *PostgresStore) Close() error {
	if s.lock != nil {
		s.lock.ExecContext(context.Background(), "SELECT pg_advisory_unlock(718890246235)")
		s.lock.Close()
	}
	return s.db.Close()
}
func (s *PostgresStore) Load() ([]Entity, error) {
	rows, err := s.db.Query("SELECT kind,id,data FROM mmex_v2_entities")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		var x Entity
		if err = rows.Scan(&x.Kind, &x.ID, &x.Data); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *PostgresStore) Commit(entities []Entity, history []HistoryWrite) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.lock.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, v := range entities {
		if v.Delete {
			_, err = tx.ExecContext(ctx, "DELETE FROM mmex_v2_entities WHERE kind=$1 AND id=$2", v.Kind, v.ID)
		} else {
			_, err = tx.ExecContext(ctx, "INSERT INTO mmex_v2_entities(kind,id,data) VALUES($1,$2,$3::jsonb) ON CONFLICT(kind,id) DO UPDATE SET data=excluded.data", v.Kind, v.ID, string(v.Data))
		}
		if err != nil {
			return err
		}
	}
	// 一条 UNNEST 覆盖本批次；不逐点往返数据库，不设上传条数配额。
	for _, h := range history {
		if len(h.Points) == 0 {
			continue
		}
		times := make([]int64, len(h.Points))
		values := make([]int64, len(h.Points))
		for i, p := range h.Points {
			times[i], values[i] = p.T, p.P
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO mmex_v2_history(stock_id,time_ms,action_points) SELECT $1,t,p FROM unnest($2::bigint[],$3::bigint[]) AS x(t,p) ON CONFLICT(stock_id,time_ms) DO UPDATE SET action_points=excluded.action_points WHERE mmex_v2_history.action_points<>excluded.action_points`, h.StockID, pgArray(times), pgArray(values))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
func pgArray(values []int64) string {
	b := make([]byte, 0, len(values)*12)
	b = append(b, '{')
	for i, v := range values {
		if i > 0 {
			b = append(b, ',')
		}
		b = strconv.AppendInt(b, v, 10)
	}
	b = append(b, '}')
	return string(b)
}
func (s *PostgresStore) Chart(id, from, to, interval int64) ([]Candle, error) {
	rows, err := s.TimedChart(id, from, to, interval)
	if err != nil {
		return nil, err
	}
	out := []Candle{}
	for _, row := range rows {
		out = append(out, row.Candle)
	}
	return out, nil
}
func (s *PostgresStore) TimedChart(id, from, to, interval int64) ([]TimedCandle, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `WITH buckets AS (SELECT ((time_ms+CASE WHEN $4::bigint>=86400000 THEN 28800000 ELSE 0 END)/$4::bigint)*$4::bigint-CASE WHEN $4::bigint>=86400000 THEN 28800000 ELSE 0 END AS bucket,min(time_ms) AS first_time,max(time_ms) AS last_time,max(action_points) AS high,min(action_points) AS low,count(*) AS samples FROM mmex_v2_history WHERE stock_id=$1 AND time_ms>=$2 AND time_ms<=$3 GROUP BY ((time_ms+CASE WHEN $4::bigint>=86400000 THEN 28800000 ELSE 0 END)/$4::bigint)*$4::bigint-CASE WHEN $4::bigint>=86400000 THEN 28800000 ELSE 0 END) SELECT b.bucket,o.action_points*100,b.high*100,b.low*100,c.action_points*100,b.samples,b.first_time,b.last_time FROM buckets b JOIN mmex_v2_history o ON o.stock_id=$1 AND o.time_ms=b.first_time JOIN mmex_v2_history c ON c.stock_id=$1 AND c.time_ms=b.last_time ORDER BY b.bucket`, id, from, to, interval)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TimedCandle{}
	for rows.Next() {
		var c TimedCandle
		if err = rows.Scan(&c.T, &c.O, &c.H, &c.L, &c.C, &c.N, &c.FirstAt, &c.LastAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *PostgresStore) Manifest(id int64) ([]Month, error) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT month,count,digest,dirty FROM mmex_v2_months WHERE stock_id=$1 ORDER BY month FOR UPDATE", id)
	if err != nil {
		return nil, err
	}
	type summary struct {
		month Month
		dirty bool
	}
	summaries := []summary{}
	for rows.Next() {
		var x summary
		if err = rows.Scan(&x.month.Month, &x.month.Count, &x.month.Digest, &x.dirty); err != nil {
			rows.Close()
			return nil, err
		}
		summaries = append(summaries, x)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	out := make([]Month, 0, len(summaries))
	for _, x := range summaries {
		if x.dirty {
			start, err := time.ParseInLocation("2006-01", x.month.Month, shanghai)
			if err != nil {
				return nil, err
			}
			end := start.AddDate(0, 1, 0)
			points, err := tx.QueryContext(ctx, "SELECT time_ms,action_points FROM mmex_v2_history WHERE stock_id=$1 AND time_ms>=$2 AND time_ms<$3 ORDER BY time_ms", id, start.UnixMilli(), end.UnixMilli())
			if err != nil {
				return nil, err
			}
			hasher := sha256.New()
			x.month.Count = 0
			for points.Next() {
				var t, p int64
				if err = points.Scan(&t, &p); err != nil {
					points.Close()
					return nil, err
				}
				fmt.Fprintf(hasher, "%d:%d\n", t, p)
				x.month.Count++
			}
			if err = points.Err(); err != nil {
				points.Close()
				return nil, err
			}
			points.Close()
			x.month.Digest = hex.EncodeToString(hasher.Sum(nil))
			if _, err = tx.ExecContext(ctx, "UPDATE mmex_v2_months SET count=$3,digest=$4,dirty=false WHERE stock_id=$1 AND month=$2", id, x.month.Month, x.month.Count, x.month.Digest); err != nil {
				return nil, err
			}
		}
		out = append(out, x.month)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}
