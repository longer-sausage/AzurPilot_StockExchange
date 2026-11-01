package exchange

import (
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresPersistenceRollbackHistoryAndExclusiveProcess(t *testing.T) {
	dsn := os.Getenv("EXCHANGE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("未配置 EXCHANGE_TEST_DATABASE_URL；真实 Postgres 集成检查未执行")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.Contains(parsed.Path, "_test") || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		t.Fatal("集成测试只允许回环地址、名称含 _test 的隔离数据库")
	}
	store, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if store != nil {
			store.Close()
		}
	}()
	if _, err = store.db.Exec("ALTER TABLE mmex_v2_entities DROP CONSTRAINT IF EXISTS injected_failure"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec("TRUNCATE mmex_v2_entities,mmex_v2_history,mmex_v2_months"); err != nil {
		t.Fatal(err)
	}
	if duplicate, err := OpenPostgres(dsn); err == nil {
		duplicate.Close()
		t.Fatal("second process acquired same database")
	}
	f := newFixture(t)
	entities, _ := f.store.Load()
	history := []HistoryWrite{}
	for id, values := range f.store.history {
		points := []Point{}
		for ts, p := range values {
			points = append(points, Point{ts, p})
		}
		history = append(history, HistoryWrite{id, points})
	}
	if err = store.Commit(entities, history); err != nil {
		t.Fatal(err)
	}
	f.e, err = NewEngine(store, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	issuer := f.account("Pg发行者")
	trader := f.account("Pg交易者")
	f.bind(issuer)
	f.order(trader, "short", 5, 100000, "pg-open-short")
	f.now = f.now.Add(time.Minute)
	f.report(1100, false)
	p, err := f.e.Preview(trader.ID, OrderInput{Stock: "1", Side: "cover", Quantity: 5, Price: 110000, ClientID: "pg-close-short"})
	if err != nil {
		t.Fatal(err)
	}
	beforeAccounts := string(encode(f.e.accounts))
	beforeOrders := string(encode(f.e.orders))
	beforeStock := string(encode(f.e.stocks))
	if _, err = store.db.Exec("ALTER TABLE mmex_v2_entities ADD CONSTRAINT injected_failure CHECK (kind <> 'account') NOT VALID"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.Confirm(trader.ID, p.PreviewID); err == nil {
		t.Fatal("expected genuine Postgres constraint failure")
	}
	if string(encode(f.e.accounts)) != beforeAccounts || string(encode(f.e.orders)) != beforeOrders || string(encode(f.e.stocks)) != beforeStock {
		t.Fatal("database abort leaked in-memory mutation")
	}
	disk, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, entity := range disk {
		if entity.Kind == "order" && strings.Contains(string(entity.Data), "pg-close-short") {
			t.Fatal("failed transaction persisted order")
		}
	}
	if _, err = store.db.Exec("ALTER TABLE mmex_v2_entities DROP CONSTRAINT injected_failure"); err != nil {
		t.Fatal(err)
	}
	o, err := f.e.Confirm(trader.ID, p.PreviewID)
	if err != nil {
		t.Fatal(err)
	}
	if o.Filled != 5 {
		t.Fatal("retry failed")
	}
	r := f.instance
	r.Username = ""
	r.Backfill = true
	r.Points = []Point{{f.now.UnixMilli() - 3600000, 700}, {f.now.UnixMilli(), 1200}, {f.now.UnixMilli(), 1300}}
	if _, err = f.e.InstanceReport(f.sign("report", r)); err != nil {
		t.Fatal(err)
	}
	candles, _, err := f.e.Chart(1, f.now.UnixMilli()-7200000, f.now.UnixMilli(), 60)
	if err != nil || len(candles) != 2 || candles[1].C != 130000 {
		t.Fatal("Postgres OHLC/correction failure", candles, err)
	}
	manifest, err := f.e.InstanceManifest(f.sign("manifest", f.instance))
	if err != nil || len(manifest) != 1 || manifest[0].Count != 3 {
		t.Fatal("Postgres manifest failure", manifest, err)
	}
	// 真实数据库故障必须一并回滚融资余额、T+N 批次、未交收现金、行情与历史。
	f.atShanghai("2026-11-02 10:00:00")
	f.profile("hk")
	f.report(1000, false)
	buyPreview, err := f.e.Preview(trader.ID, OrderInput{Stock: "1", Side: "buy", Quantity: 200, Price: 100000, Leverage: 2, ClientID: "pg-financed-hk-buy"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.Confirm(trader.ID, buyPreview.PreviewID); err != nil {
		t.Fatal(err)
	}
	f.order(trader, "sell", 100, 100000, "pg-unsettled-hk-sell")
	if f.e.accounts[trader.ID].Positions[1].Loan <= 0 || len(f.e.accounts[trader.ID].Settlements) != 1 {
		t.Fatal("integration fixture has no financing/settlement")
	}
	settings := f.e.Settings()
	settings.DelistThreshold = 500
	if err = f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	beforeAccounts = string(encode(f.e.accounts))
	beforeOrders = string(encode(f.e.orders))
	beforeStock = string(encode(f.e.stocks))
	var historyCount int64
	if err = store.db.QueryRow("SELECT count(*) FROM mmex_v2_history").Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec("ALTER TABLE mmex_v2_entities ADD CONSTRAINT injected_failure CHECK (kind <> 'account') NOT VALID"); err != nil {
		t.Fatal(err)
	}
	zero := f.instance
	zero.Username = ""
	zero.Points = []Point{{f.now.UnixMilli(), 0}}
	if _, err = f.e.InstanceReport(f.sign("report", zero)); err == nil {
		t.Fatal("expected genuine financing/delisting transaction failure")
	}
	if string(encode(f.e.accounts)) != beforeAccounts || string(encode(f.e.orders)) != beforeOrders || string(encode(f.e.stocks)) != beforeStock {
		t.Fatal("Postgres abort leaked lots/settlements/financing")
	}
	var afterCount int64
	if err = store.db.QueryRow("SELECT count(*) FROM mmex_v2_history").Scan(&afterCount); err != nil || afterCount != historyCount {
		t.Fatal("history committed outside failed ledger transaction", err)
	}
	if _, err = store.db.Exec("ALTER TABLE mmex_v2_entities DROP CONSTRAINT injected_failure"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.InstanceReport(f.sign("report", zero)); err != nil {
		t.Fatal(err)
	}
	expected := string(encode(f.e.accounts))
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = nil
	restarted, err := Open(dsn, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if string(encode(restarted.accounts)) != expected {
		t.Fatal("Postgres restart lost positions, cash or binding")
	}
	again, err := restarted.Confirm(trader.ID, p.PreviewID)
	if err != nil || again.ID != o.ID {
		t.Fatal("Postgres restart lost idempotency receipt", err)
	}
}
