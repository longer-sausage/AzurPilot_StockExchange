package exchange

import (
	"strings"
	"testing"
	"time"
)

func TestBotRegistrationOnboardingRetryAndRollback(t *testing.T) {
	f := newFixture(t)
	secret := strings.Repeat("s", 64)
	before, changes := f.e.Changes()
	f.store.fail = true
	_, err := f.e.BotRegister("app:onboard", "新用户", "register-onboard", secret)
	if err == nil {
		t.Fatal("registration survived failed commit")
	}
	after, _ := f.e.Changes()
	if before != after || len(f.e.accounts) != 0 {
		t.Fatal("failed registration changed memory")
	}
	select {
	case <-changes:
		t.Fatal("failed registration notified")
	default:
	}
	f.store.fail = false
	out, err := f.e.BotRegister("app:onboard", "新用户", "register-onboard", secret)
	if err != nil {
		t.Fatal(err)
	}
	id := out["accountId"].(int64)
	password := out["password"].(string)
	if !validPassword(password) {
		t.Fatal("weak initial password")
	}
	retry, err := f.e.BotRegister("app:onboard", "新用户", "register-onboard", secret)
	if err != nil || retry["password"] != password || len(f.e.accounts) != 1 {
		t.Fatal("registration not idempotent", err)
	}
	_, err = f.e.Preview(id, OrderInput{Stock: "1", Side: "buy", Quantity: 1, Price: 100000, ClientID: "unready-preview"})
	mustCode(t, err, "PASSWORD_CHANGE_REQUIRED")
	if _, err = f.e.ChangePassword(id, password, password); err == nil {
		t.Fatal("initial password accepted as new")
	}
	if _, err = f.e.ChangePassword(id, password, "NewStrong7!Password"); err != nil {
		t.Fatal(err)
	}
	_, err = f.e.Preview(id, OrderInput{Stock: "1", Side: "buy", Quantity: 1, Price: 100000, ClientID: "unbound-preview"})
	mustCode(t, err, "BINDING_REQUIRED")
	mustCode(t, f.e.SetWatchlist(id, 1, true), "BINDING_REQUIRED")
	retry, err = f.e.BotRegister("app:onboard", "新用户", "register-onboard", secret)
	if err != nil || retry["password"] != nil {
		t.Fatal("old initial password returned after change")
	}
	if _, err = f.e.Bind(id, f.e.stocks[1].BindingCode); err != nil {
		t.Fatal(err)
	}
	if err = f.e.Ready(id); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewEngine(f.store, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	if err = restarted.Ready(id); err != nil {
		t.Fatal(err)
	}
	for _, entity := range f.store.entities {
		if strings.Contains(string(entity.Data), password) {
			t.Fatal("plaintext initial password persisted")
		}
	}
}
func TestDeletedStockAtomicSettlementBindingAndRetiredInstance(t *testing.T) {
	f := newFixture(t)
	issuer := f.account("删除发行账户")
	f.bind(issuer)
	buyer := f.account("删除多头账户")
	shorter := f.account("删除空头账户")
	f.order(buyer, "buy", 2, 100000, "delete-buy-test")
	f.order(shorter, "short", 2, 100000, "delete-short-test")
	f.now = f.now.Add(time.Minute)
	f.report(1100, false)
	pending := f.order(buyer, "buy", 1, 1, "delete-pending-test")
	before := string(encode(map[string]any{"accounts": f.e.accounts, "stocks": f.e.stocks, "orders": f.e.orders, "trades": f.e.trades}))
	version, changes := f.e.Changes()
	f.store.fail = true
	if err := f.e.DeleteStock(1); err == nil {
		t.Fatal("deletion committed through failure")
	}
	f.store.fail = false
	after := string(encode(map[string]any{"accounts": f.e.accounts, "stocks": f.e.stocks, "orders": f.e.orders, "trades": f.e.trades}))
	if before != after {
		t.Fatal("failed deletion changed money, holdings, or trade history")
	}
	select {
	case <-changes:
		t.Fatal("failed deletion broadcast")
	default:
	}
	if current, _ := f.e.Changes(); current != version {
		t.Fatal("failed deletion advanced version")
	}
	cash := f.e.accounts[issuer.ID].Cash
	if err := f.e.DeleteStock(1); err != nil {
		t.Fatal(err)
	}
	if f.e.accounts[issuer.ID].BoundStockID != 0 || f.e.stocks[1].AccountID != 0 || f.e.stocks[1].DeletedAt == 0 {
		t.Fatal("deletion did not revoke binding")
	}
	mustCode(t, f.e.Ready(issuer.ID), "BINDING_REQUIRED")
	if f.e.accounts[issuer.ID].Cash != cash+20000 {
		t.Fatal("actual short loss compensation missing")
	}
	if f.e.accounts[buyer.ID].Positions[1].Quantity != 0 || f.e.accounts[shorter.ID].Positions[1].ShortQuantity != 0 || f.e.stocks[1].Outstanding != 0 || f.e.stocks[1].Borrowed != 0 {
		t.Fatal("deletion left open position/inventory")
	}
	if f.e.orders[pending.ID].Status != "cancelled" {
		t.Fatal("deletion left pending order")
	}
	_, err := f.e.InstanceStatus(f.sign("status", f.instance))
	mustCode(t, err, "STOCK_DELETED")
	_, err = f.e.InstanceRegister(f.sign("register", f.instance))
	mustCode(t, err, "STOCK_DELETED")
	for _, a := range f.e.Seasons()["rankings"].([]SeasonRank) {
		if a.AccountID == issuer.ID {
			t.Fatal("unbound account still participates")
		}
	}
	committed := string(encode(f.e.accounts))
	if err = f.e.DeleteStock(1); err != nil || string(encode(f.e.accounts)) != committed {
		t.Fatal("repeated deletion changed settlement")
	}
	restarted, err := NewEngine(f.store, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	f.e = restarted
	_, err = f.e.InstanceReport(f.sign("report", InstanceRequest{InstanceID: f.instance.InstanceID, PublicKey: f.instance.PublicKey, Points: []Point{{f.now.UnixMilli(), 1000}}}))
	mustCode(t, err, "STOCK_DELETED")
}
func TestWatchlistPersistencePrivateAndExactTradeEvents(t *testing.T) {
	f := newFixture(t)
	a := f.account("自选账户")
	if err := f.e.SetWatchlist(a.ID, 1, true); err != nil {
		t.Fatal(err)
	}
	version, _ := f.e.Changes()
	if err := f.e.SetWatchlist(a.ID, 1, true); err != nil {
		t.Fatal(err)
	}
	if after, _ := f.e.Changes(); after != version {
		t.Fatal("idempotent watchlist update wrote again")
	}
	f.order(a, "buy", 2, 100000, "trade-events-buy")
	f.now = f.now.Add(time.Minute)
	f.report(1200, false)
	f.order(a, "sell", 1, 120000, "trade-events-sell")
	if len(f.e.trades) != 2 || f.e.accounts[a.ID].Realized != 20000 {
		t.Fatal("actual fills/realized PnL not recorded")
	}
	data, err := f.e.StockData(1, 0, f.now.UnixMilli())
	if err != nil || len(data["trades"].([]*Trade)) != 2 {
		t.Fatal("public monthly prints missing", err)
	}
	public, _ := f.e.Market()
	if strings.Contains(string(public), "watchlist") || strings.Contains(string(public), "identityCode") {
		t.Fatal("private watchlist/identity leaked into market")
	}
	restarted, err := NewEngine(f.store, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.accounts[a.ID].Watchlist) != 1 || len(restarted.trades) != 2 {
		t.Fatal("watchlist/prints lost after restart")
	}
}
func TestMarketStopAndDAYExpiry(t *testing.T) {
	f := newFixture(t)
	a := f.account("高级委托账户")
	p, err := f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "buy", Kind: "market", TimeInForce: "GTC", Quantity: 1, ClientID: "market-kind-test"})
	if err != nil {
		t.Fatal(err)
	}
	o, err := f.e.Confirm(a.ID, p.PreviewID)
	if err != nil || o.Filled != 1 {
		t.Fatal("market order not filled", err)
	}
	p, err = f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "buy", Kind: "stop", TimeInForce: "DAY", Price: 110000, Quantity: 1, ClientID: "stop-kind-test"})
	if err != nil {
		t.Fatal(err)
	}
	o, err = f.e.Confirm(a.ID, p.PreviewID)
	if err != nil || o.Filled != 0 || o.ExpiresAt == 0 {
		t.Fatal("stop triggered before threshold", err)
	}
	f.now = f.now.Add(time.Hour)
	f.report(1200, false)
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if f.e.orders[o.ID].Filled != 1 || !f.e.orders[o.ID].Triggered {
		t.Fatal("stop did not execute at hourly auction")
	}
	p, err = f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "buy", Kind: "limit", TimeInForce: "DAY", Price: 1, Quantity: 1, ClientID: "day-kind-test"})
	if err != nil {
		t.Fatal(err)
	}
	o, err = f.e.Confirm(a.ID, p.PreviewID)
	if err != nil {
		t.Fatal(err)
	}
	f.now = time.UnixMilli(o.ExpiresAt)
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if f.e.orders[o.ID].Status != "expired" || f.e.accounts[a.ID].Reserved != 0 {
		t.Fatal("DAY expiry did not release funds")
	}
}

func (s *memoryStore) TimedChart(id, from, to, step int64) ([]TimedCandle, error) {
	candles, err := s.Chart(id, from, to, step)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TimedCandle, len(candles))
	byTime := map[int64]int{}
	seen := map[int64]bool{}
	for i, c := range candles {
		out[i].Candle = c
		byTime[c.T] = i
	}
	offset := int64(0)
	if step >= 86400000 {
		offset = 28800000
	}
	for stamp := range s.history[id] {
		if stamp < from || stamp > to {
			continue
		}
		bucket := (stamp+offset)/step*step - offset
		i, ok := byTime[bucket]
		if !ok {
			continue
		}
		if !seen[bucket] {
			out[i].FirstAt, out[i].LastAt = stamp, stamp
			seen[bucket] = true
		} else {
			out[i].FirstAt = min(out[i].FirstAt, stamp)
			out[i].LastAt = max(out[i].LastAt, stamp)
		}
	}
	return out, nil
}
func TestChartUsesActualEventOrderAndSignedHistoryReconcile(t *testing.T) {
	f := newFixture(t)
	a := f.account("精确图表账户")
	settings := f.e.Settings()
	settings.PremiumPPM = 100000
	if err := f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(30 * time.Second)
	o := f.order(a, "buy", 1, 200000, "precise-chart-buy")
	bars, err := f.e.DetailBars(1, f.now.Add(-time.Minute).UnixMilli(), f.now.UnixMilli(), 5)
	if err != nil || len(bars) != 1 || bars[0].Close != o.AveragePrice || bars[0].Volume != 1 || bars[0].Samples != 1 {
		t.Fatal("trade after quote did not set exact candle close", bars, err)
	}
	f.now = f.now.Add(time.Minute)
	f.order(a, "buy", 1, 200000, "trade-only-chart")
	bars, err = f.e.DetailBars(1, f.now.UnixMilli(), f.now.UnixMilli()+1000, 1)
	if err != nil || len(bars) != 1 || bars[0].Samples != 0 || bars[0].Volume != 1 {
		t.Fatal("trade-only candle missing/fabricated quote", bars, err)
	}
	months, err := f.e.InstanceManifest(f.sign("manifest", f.instance))
	if err != nil {
		t.Fatal(err)
	}
	req := f.instance
	req.Months = months
	bad := req
	bad.Months = append([]Month{}, months...)
	bad.Months[0].Count++
	_, err = f.e.InstanceReconcile(f.sign("reconcile", bad))
	mustCode(t, err, "HISTORY_MISMATCH")
	if f.e.stocks[1].HistoryCheckedAt != 0 {
		t.Fatal("mismatched manifest marked checked")
	}
	f.store.fail = true
	if _, err = f.e.InstanceReconcile(f.sign("reconcile", req)); err == nil {
		t.Fatal("reconcile persisted through failed commit")
	}
	f.store.fail = false
	if _, err = f.e.InstanceReconcile(f.sign("reconcile", req)); err != nil {
		t.Fatal(err)
	}
	version, _ := f.e.Changes()
	if _, err = f.e.InstanceReconcile(f.sign("reconcile", req)); err != nil {
		t.Fatal(err)
	}
	if after, _ := f.e.Changes(); after != version {
		t.Fatal("same manifest acknowledged twice")
	}
}
func TestForcedSettlementRecordsOnlyCollectedFees(t *testing.T) {
	f := newFixture(t)
	a := f.account("实际收费账户")
	f.atShanghai("2026-11-02 10:00:00")
	f.report(1000, false)
	f.profile("hk")
	p, err := f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "buy", Quantity: 100, Price: 100000, Leverage: 2, ClientID: "actual-fee-buy"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.Confirm(a.ID, p.PreviewID); err != nil {
		t.Fatal(err)
	}
	cash := int64(0)
	if err = f.e.UpdateAccount(a.ID, &cash, nil); err != nil {
		t.Fatal(err)
	}
	before := f.e.accounts[a.ID].Fees
	f.now = f.now.Add(time.Minute)
	f.report(0, false)
	after := f.e.accounts[a.ID].Fees
	if after.Commission != before.Commission {
		t.Fatal("uncollected liquidation commission counted as revenue")
	}
}

func TestLegacyFeatureMigrationIsAtomicAndDoesNotInventPrints(t *testing.T) {
	f := newFixture(t)
	a := f.account("旧数据迁移账户")
	settings := f.e.Settings()
	settings.CommissionPPM = 1000
	if err := f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	o := f.order(a, "buy", 1, 100000, "legacy-order-test")
	legacy := *f.e.accounts[a.ID]
	legacy.IdentityCode = ""
	legacy.Fees = Revenue{}
	legacy.MetricsSince = 0
	legacy.LegacyMetrics = false
	f.store.entities["account:"+idstr(a.ID)] = Entity{Kind: "account", ID: idstr(a.ID), Data: encode(legacy)}
	oldOrder := *o
	oldOrder.Kind = ""
	oldOrder.TimeInForce = ""
	oldOrder.TradeRecorded = false
	f.store.entities["order:"+idstr(o.ID)] = Entity{Kind: "order", ID: idstr(o.ID), Data: encode(oldOrder)}
	meta := f.e.meta
	meta.FeatureVersion = 0
	meta.NextTrade = 0
	meta.Revenue = Revenue{}
	meta.LifetimeRevenue = Revenue{}
	f.store.entities["meta:1"] = Entity{Kind: "meta", ID: "1", Data: encode(meta)}
	for key, entity := range f.store.entities {
		if entity.Kind == "trade" {
			delete(f.store.entities, key)
		}
	}
	f.store.entities["registration:retired-key"] = Entity{Kind: "registration", ID: "retired-key", Data: []byte(`{}`)}
	before := string(encode(f.store.entities))
	f.store.fail = true
	if _, err := NewEngine(f.store, func() time.Time { return f.now }); err == nil {
		t.Fatal("migration survived failed database commit")
	}
	f.store.fail = false
	if string(encode(f.store.entities)) != before {
		t.Fatal("failed migration changed persisted entities")
	}
	restored, err := NewEngine(f.store, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	user := restored.accounts[a.ID]
	if user.Cash != legacy.Cash || user.BoundStockID != legacy.BoundStockID || user.IdentityCode == "" || !user.LegacyMetrics {
		t.Fatal("migration lost balances/binding or failed to mark statistics boundary")
	}
	if restored.orders[o.ID].LegacyFilled != o.Filled || len(restored.trades) != 0 {
		t.Fatal("legacy aggregate was lost or fabricated as prints")
	}
	if user.Fees.Commission != o.Commission {
		t.Fatal("known legacy transaction fees not restored")
	}
	f.store.entities["registration:late-key"] = Entity{Kind: "registration", ID: "late-key", Data: []byte(`{}`)}
	restored, err = NewEngine(f.store, func() time.Time { return f.now })
	if err != nil || restored.accounts[a.ID].Fees.Commission != o.Commission {
		t.Fatal("restart repeated fee migration", err)
	}
	if _, exists := f.store.entities["registration:late-key"]; exists {
		t.Fatal("retired website key survived migration")
	}
}
func TestAdminCashCompareAndSetRenameAndDisable(t *testing.T) {
	f := newFixture(t)
	a := f.account("管理变更账户")
	f.order(a, "buy", 1, 100000, "admin-holding-test")
	expected := f.e.accounts[a.ID].Cash
	wrong := expected + 1
	delta := int64(100)
	name := "ＡＢC账户"
	mustCode(t, f.e.EditAccount(a.ID, AccountEdit{Username: &name, CashAdjustment: &delta, ExpectedCash: &wrong}), "CASH_CHANGED")
	if f.e.accounts[a.ID].Username != a.Username {
		t.Fatal("failed balance comparison changed username")
	}
	if err := f.e.EditAccount(a.ID, AccountEdit{Username: &name, CashAdjustment: &delta, ExpectedCash: &expected}); err != nil {
		t.Fatal(err)
	}
	if f.e.accounts[a.ID].InitialCash != a.InitialCash {
		t.Fatal("cash adjustment changed season principal")
	}
	id, _, _, err := f.e.Login("abc账户", "NewStrong7!Password")
	if err != nil || id != a.ID {
		t.Fatal("Unicode equivalent login failed", err)
	}
	_, err = f.e.BotRegister("app:canonical-other", "ABC账户", "canonical-register", strings.Repeat("s", 64))
	mustCode(t, err, "NAME_TAKEN")
	disabled := true
	mustCode(t, f.e.EditAccount(a.ID, AccountEdit{Disabled: &disabled}), "CONFIRMATION_REQUIRED")
	if err = f.e.EditAccount(a.ID, AccountEdit{Disabled: &disabled, ConfirmDisable: true}); err != nil {
		t.Fatal(err)
	}
	if !f.e.accounts[a.ID].Disabled || f.e.accounts[a.ID].Positions[1].Quantity != 0 {
		t.Fatal("disable did not close account holdings")
	}
}
func TestResetReceiptCannotReturnSupersededPassword(t *testing.T) {
	f := newFixture(t)
	a := f.account("重置版本账户")
	secret := strings.Repeat("s", 64)
	first, err := f.e.ResetPassword(a.OfficialUserID, "reset-version-first", secret)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.e.ResetPassword(a.OfficialUserID, "reset-version-first", secret)
	if err != nil || again != first {
		t.Fatal("same reset not idempotent")
	}
	if _, err = f.e.ResetPassword(a.OfficialUserID, "reset-version-second", secret); err != nil {
		t.Fatal(err)
	}
	_, err = f.e.ResetPassword(a.OfficialUserID, "reset-version-first", secret)
	mustCode(t, err, "RESET_SUPERSEDED")
}

func TestPortfolioETagIncludesUnpostedInterest(t *testing.T) {
	f := newFixture(t)
	issuer := f.account("估息发行账户")
	f.bind(issuer)
	a := f.account("估息交易账户")
	settings := f.e.Settings()
	settings.BorrowAnnualPPM = 1000000
	if err := f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	f.order(a, "short", 10, 100000, "tag-unposted-fee")
	before, err := f.e.PortfolioTag(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Hour)
	after, err := f.e.PortfolioTag(a.ID)
	if err != nil || before == after {
		t.Fatal("stale account ETag hid newly accrued fee", err)
	}
}
