package exchange

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDelistingStartsOnFifthShanghaiDate(t *testing.T) {
	zone, _ := loadLocation("Asia/Shanghai")
	f := setupAt(t, time.Date(2026, 10, 4, 23, 59, 0, 0, zone))
	boundary, _, err := f.e.Register("阈值猫猫", "test-password-123", 500, f.now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(30 * time.Second)
	if err = f.e.Upload(f.upload, 499, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	if f.e.players[f.owner.ID].Delisted {
		t.Fatal("5 日前不应退市")
	}
	f.now = f.now.Add(30 * time.Second)
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if !f.e.players[f.owner.ID].Delisted || f.e.players[boundary.ID].Delisted {
		t.Fatal("5 日零点应只退市低于阈值的股票，等于阈值仍上市")
	}
	revision := f.e.state.Revision
	if err = f.e.Tick(); err != nil || f.e.state.Revision != revision || len(f.e.delistCandidates) != 0 {
		t.Fatal("退市处理应幂等且不再扫描已退市股票", err)
	}
}

func TestDelistingWaitsForSeasonStart(t *testing.T) {
	f := setup(t)
	starts := f.now.Add(24 * time.Hour)
	if err := f.e.transaction(func() error { f.e.state.Season.StartsAt = starts.Unix(); return nil }); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	if err := f.e.Upload(f.upload, 499, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	if f.e.players[f.owner.ID].Delisted {
		t.Fatal("赛季尚未开赛时不应退市")
	}
	f.now = starts
	if err := f.e.Tick(); err != nil || !f.e.players[f.owner.ID].Delisted {
		t.Fatal("赛季开赛后应判断退市", err)
	}
}

func TestDelistingClosesOnlyAffectedStockAndPersistsUntilNextMonth(t *testing.T) {
	f := setup(t)
	s := f.e.Settings()
	s.Active.SellDelayDays = 2
	s.Active.CashDelayDays = 2
	if err := f.e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	other, _, err := f.e.Register("其他股票", "test-password-123", 2000, f.now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	short, _, err := f.e.Register("空头猫猫", "test-password-123", 2000, f.now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	buy := order(f, "buy", "market", 100, 0, "delist_long_01")
	buy.Leverage = 2
	if _, err = f.e.Submit(f.trader.ID, buy); err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.Submit(short.ID, order(f, "short", "market", 50, 0, "delist_short_01")); err != nil {
		t.Fatal(err)
	}
	unrelated := order(f, "buy", "market", 5, 0, "unrelated_long_01")
	unrelated.StockID = other.ID
	if _, err = f.e.Submit(f.trader.ID, unrelated); err != nil {
		t.Fatal(err)
	}
	unrelated.ClientID, unrelated.Kind, unrelated.Limit = "unrelated_pending_01", "limit", 100000
	unrelatedPending, err := f.e.Submit(f.trader.ID, unrelated)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := f.e.Submit(f.trader.ID, order(f, "buy", "limit", 2, 45000, "delist_pending_01"))
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Hour)
	longBefore, _ := f.e.Account(f.trader.ID)
	shortBefore, _ := f.e.Account(short.ID)
	if err = f.e.Upload(f.upload, 400, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	longAfter, _ := f.e.Account(f.trader.ID)
	shortAfter, _ := f.e.Account(short.ID)
	if longAfter.Player.Positions[f.owner.ID] != nil || shortAfter.Player.Positions[f.owner.ID] != nil || longAfter.FinancingDebt != 0 || longAfter.Unsettled != 0 {
		t.Fatal("退市应越过 T+N 平掉多空仓并还清相关融资")
	}
	if longAfter.Player.Positions[other.ID].Quantity != 5 || longAfter.Player.Cash != longBefore.Player.Cash-longBefore.FinancingAccrued+40000*100-5000000-FeesFor(s.Active, "sell", 40000*100).Total() {
		t.Fatal("退市结算金额错误或影响其他股票持仓")
	}
	if shortAfter.Player.Cash != shortBefore.Player.Cash-shortBefore.BorrowAccrued-40000*50-FeesFor(s.Active, "cover", 40000*50).Total() {
		t.Fatal("退市空仓结算应按更新前价格计息，再按触发价回补")
	}
	for _, o := range longAfter.Player.Orders {
		if o.ID == pending.ID && (o.Status != "cancelled" || o.Reserved != 0 || !strings.Contains(o.Reason, "退市")) {
			t.Fatal("触发退市的低价不能先成交挂单")
		}
		if o.ID == unrelatedPending.ID && o.Status != "pending" {
			t.Fatal("不应撤销其他股票挂单")
		}
	}
	last := longAfter.Player.Orders[len(longAfter.Player.Orders)-1]
	if last.Status != "filled" || last.Side != "sell" || last.Price != 40000 || !strings.Contains(last.Reason, "退市") {
		t.Fatal("退市强平成交和原因应保留")
	}
	_, err = f.e.Submit(f.trader.ID, order(f, "buy", "market", 1, 0, "delisted_reject_01"))
	requireCode(t, err, "STOCK_DELISTED")
	if _, err = f.e.Login(f.owner.Username, "test-password-123"); err != nil {
		t.Fatal("股票退市不应停用玩家账户", err)
	}
	// 价格恢复、降低阈值以及重启都不能重新上市本月股票。
	f.now = f.now.Add(time.Minute)
	if err = f.e.Upload(f.upload, 1500, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	s.DelistThreshold = 0
	if err = f.e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	f.e.Close()
	f.e, err = Open(f.path, func() time.Time { return f.now })
	if err != nil || !f.e.players[f.owner.ID].Delisted || f.e.players[f.owner.ID].Disabled {
		t.Fatal("退市状态未持久保存或与账户停用混淆", err)
	}
	f.now = time.Date(2026, 11, 1, 1, 0, 0, 0, time.UTC)
	if err = f.e.Tick(); err != nil || f.e.players[f.owner.ID].Delisted {
		t.Fatal("次月应重置退市状态", err)
	}
	s.DelistThreshold = 500
	if err = f.e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	if err = f.e.Upload(f.upload, 499, f.now.Unix()); err != nil || f.e.players[f.owner.ID].Delisted {
		t.Fatal("次月 5 日前不应判断低行动力退市", err)
	}
	f.now = time.Date(2026, 11, 5, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
	if err = f.e.Tick(); err != nil || !f.e.players[f.owner.ID].Delisted {
		t.Fatal("下月开赛后应重新判断退市资格", err)
	}
}

func TestHistoryDelistingUsesOnlyCurrentQuote(t *testing.T) {
	f := setup(t)
	_, binding := historyIdentity(t, f)
	historySend(t, f, binding, []HistoryPoint{{f.now.Add(-48 * time.Hour).UnixMilli(), 100}})
	if f.e.players[f.owner.ID].Delisted {
		t.Fatal("迟到的旧低价不能重放退市")
	}
	f.now = f.now.Add(time.Minute)
	historySend(t, f, binding, []HistoryPoint{{f.now.UnixMilli(), 499}})
	if !f.e.players[f.owner.ID].Delisted {
		t.Fatal("新鲜历史记录更新实时报价时应判断退市")
	}
	f.now = f.now.Add(time.Minute)
	historySend(t, f, binding, []HistoryPoint{{f.now.UnixMilli(), 1000}})
	if !f.e.players[f.owner.ID].Delisted || f.e.players[f.owner.ID].Quote.Price != 100000 {
		t.Fatal("退市后仍应接收报价，但本月不得恢复上市")
	}
}

func TestDelistingRollbackOnFailedPersistence(t *testing.T) {
	f := setup(t)
	if _, err := f.e.Submit(f.trader.ID, order(f, "buy", "market", 10, 0, "delist_rollback_long")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.Submit(f.trader.ID, order(f, "buy", "limit", 2, 45000, "delist_rollback_pending")); err != nil {
		t.Fatal(err)
	}
	before, _ := f.e.Account(f.trader.ID)
	revision := f.e.state.Revision
	f.now = f.now.Add(time.Minute)
	f.e.db.Close()
	if err := f.e.Upload(f.upload, 400, f.now.Unix()); err == nil {
		t.Fatal("关闭数据库后不应成功保存")
	}
	after, _ := f.e.Account(f.trader.ID)
	if f.e.players[f.owner.ID].Delisted || f.e.players[f.owner.ID].Quote.Price != 100000 || before.Player.Cash != after.Player.Cash || after.Player.Positions[f.owner.ID].Quantity != 10 || after.Player.Orders[len(after.Player.Orders)-1].Status != "pending" || f.e.state.Revision != revision {
		t.Fatal("保存失败必须一起回滚退市状态、行情、成交及撤单")
	}
}

func TestDelistThresholdDefaultMigrationAndValidation(t *testing.T) {
	f := setup(t)
	if f.e.Settings().DelistThreshold != 500 {
		t.Fatal("默认退市阈值应为 500 点")
	}
	for _, threshold := range []int64{-1, 1000001} {
		s := f.e.Settings()
		s.DelistThreshold = threshold
		requireCode(t, f.e.SetSettings(s), "INVALID_RULES")
	}
	s := f.e.Settings()
	s.DelistThreshold = 1500
	if err := f.e.SetSettings(s); err != nil || !f.e.players[f.owner.ID].Delisted || f.e.players[f.trader.ID].Delisted {
		t.Fatal("自定义阈值应即时判断上市资格", err)
	}
	s.DelistThreshold = 0
	if err := f.e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	f.e.Close()
	var err error
	f.e, err = Open(f.path, func() time.Time { return f.now })
	if err != nil || f.e.Settings().DelistThreshold != 0 {
		t.Fatal("显式关闭退市规则应在重启后保留", err)
	}
	var raw string
	if err = f.e.db.QueryRow("SELECT data FROM metadata WHERE id=1").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var state, settings map[string]json.RawMessage
	if err = json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(state["settings"], &settings); err != nil {
		t.Fatal(err)
	}
	delete(settings, "delistThreshold")
	state["settings"], _ = json.Marshal(settings)
	data, _ := json.Marshal(state)
	if _, err = f.e.db.Exec("UPDATE metadata SET data=? WHERE id=1", string(data)); err != nil {
		t.Fatal(err)
	}
	f.e.Close()
	f.e, err = Open(f.path, func() time.Time { return f.now })
	if err != nil || f.e.Settings().DelistThreshold != 500 {
		t.Fatal("旧数据库缺少阈值时应迁移为 500", err)
	}
}
