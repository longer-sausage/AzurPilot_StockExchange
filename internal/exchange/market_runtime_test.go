package exchange

import (
	"testing"
	"time"
)

func (f *fixture) profile(id string) {
	f.t.Helper()
	settings := f.e.Settings()
	for _, r := range settings.MarketPresets {
		if r.ID == id {
			settings.ActiveMarket = r
			break
		}
	}
	if err := f.e.SetSettings(settings); err != nil {
		f.t.Fatal(err)
	}
}
func (f *fixture) atShanghai(date string) {
	f.t.Helper()
	location, _ := MarketLocation("Asia/Shanghai")
	now, err := time.ParseInLocation("2006-01-02 15:04:05", date, location)
	if err != nil {
		f.t.Fatal(err)
	}
	f.now = now
}
func TestAShareRuntimeLotsHoursTaxAndNextDayAvailability(t *testing.T) {
	f := newFixture(t)
	f.atShanghai("2026-11-09 10:00:00")
	f.profile("a-share")
	f.report(1000, false)
	a := f.account("A股交易账户")
	_, err := f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "buy", Quantity: 1, Price: 100000, ClientID: "a-invalid-lot"})
	mustCode(t, err, "INVALID_LOT")
	buy := f.order(a, "buy", 100, 100000, "a-buy-hundred")
	if buy.Filled != 100 || buy.Commission != 3000 || buy.Stamp != 0 || buy.Tax != 0 || buy.AveragePrice != 100000 {
		t.Fatalf("A-share fees/premium %+v", buy)
	}
	_, err = f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "sell", Quantity: 100, Price: 100000, ClientID: "a-same-day-sell"})
	mustCode(t, err, "INSUFFICIENT_POSITION")
	before, _ := f.e.PortfolioTag(a.ID)
	f.atShanghai("2026-11-10 00:00:00")
	after, _ := f.e.PortfolioTag(a.ID)
	if before == after {
		t.Fatal("T+1 lot maturity did not invalidate account ETag")
	}
	f.atShanghai("2026-11-10 10:00:00")
	f.report(1000, false)
	sell := f.order(a, "sell", 100, 100000, "a-next-day-sell")
	if sell.Filled != 100 || sell.Stamp != 5000 || sell.Commission != 3000 || sell.Tax != 0 {
		t.Fatal("A-share sell fee runtime", sell)
	}
	if f.e.accounts[a.ID].Cash != f.e.Settings().InitialCash-11000 {
		t.Fatal("A-share fees not charged exactly")
	}
	f.atShanghai("2026-11-14 10:00:00")
	_, err = f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "buy", Quantity: 100, Price: 100000, ClientID: "a-weekend-closed"})
	mustCode(t, err, "MARKET_CLOSED")
}
func TestHKRuntimeSettlementAndTimeSensitivePortfolioETag(t *testing.T) {
	f := newFixture(t)
	f.atShanghai("2026-11-06 10:00:00")
	f.profile("hk")
	f.report(1000, false)
	a := f.account("港股交收账户")
	buy := f.order(a, "buy", 100, 100000, "hk-buy-hundred")
	if buy.Stamp != 10000 || buy.Levy != 850 || buy.Commission != 3000 || buy.Tax != 0 {
		t.Fatal(buy)
	}
	sell := f.order(a, "sell", 100, 100000, "hk-sell-hundred")
	if sell.Stamp != 10000 || sell.Levy != 850 || sell.Commission != 3000 {
		t.Fatal(sell)
	}
	portfolio, err := f.e.Portfolio(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	account := portfolio["account"].(map[string]any)
	if len(f.e.accounts[a.ID].Settlements) != 1 || account["availableCash"].(int64) != f.e.Settings().InitialCash-10_013_850 {
		t.Fatal("unsettled sale proceeds became spendable", account)
	}
	before, _ := f.e.PortfolioTag(a.ID)
	f.atShanghai("2026-11-10 00:00:00")
	after, _ := f.e.PortfolioTag(a.ID)
	if before == after {
		t.Fatal("cash maturity did not invalidate ETag")
	}
	portfolio, _ = f.e.Portfolio(a.ID)
	account = portfolio["account"].(map[string]any)
	if account["availableCash"].(int64) != f.e.accounts[a.ID].Cash || len(account["settlements"].([]Settlement)) != 0 {
		t.Fatal("T+2 did not mature on second trading day", account)
	}
}
func TestFinancingRuntimeAccrualRiskZeroQuoteAndRollback(t *testing.T) {
	f := newFixture(t)
	f.atShanghai("2026-11-06 10:00:00")
	f.report(1000, false)
	a := f.account("融资运行账户")
	settings := f.e.Settings()
	settings.ActiveMarket.FinancingAnnualPPM = 1000000
	if err := f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	p, err := f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "buy", Quantity: 10, Price: 100000, Leverage: 2, ClientID: "financed-buy-ten"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Financing != 500000 {
		t.Fatal("preview financing principal missing")
	}
	if _, err = f.e.Confirm(a.ID, p.PreviewID); err != nil {
		t.Fatal(err)
	}
	position := f.e.accounts[a.ID].Positions[1]
	if position.Loan != 500000 || position.Quantity != 10 {
		t.Fatal(position)
	}
	before := f.e.accounts[a.ID].Cash
	f.now = f.now.Add(24 * time.Hour)
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if got := before - f.e.accounts[a.ID].Cash; got != 1369 {
		t.Fatalf("financing interest got %d", got)
	}
	cash := int64(0)
	if err = f.e.UpdateAccount(a.ID, &cash, nil); err != nil {
		t.Fatal(err)
	}
	settings = f.e.Settings()
	settings.DelistThreshold = 500
	if err = f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	snapshot := string(encode(f.e.accounts))
	stock := string(encode(f.e.stocks))
	f.store.fail = true
	r := f.instance
	r.Username = ""
	r.Points = []Point{{f.now.UnixMilli(), 0}}
	if _, err = f.e.InstanceReport(f.sign("report", r)); err == nil {
		t.Fatal("expected commit failure")
	}
	if snapshot != string(encode(f.e.accounts)) || stock != string(encode(f.e.stocks)) {
		t.Fatal("failed forced settlement mutated financing/lots")
	}
	f.store.fail = false
	if _, err = f.e.InstanceReport(f.sign("report", r)); err != nil {
		t.Fatal(err)
	}
	position = f.e.accounts[a.ID].Positions[1]
	if position.Quantity != 0 || position.Loan != 0 || f.e.accounts[a.ID].Cash != 0 || !f.e.stocks[1].Delisted {
		t.Fatal("zero quote did not settle financing and delist safely", position)
	}
}
func TestSeasonCloseArchiveRestartAndNextMonthReset(t *testing.T) {
	f := newFixture(t)
	f.atShanghai("2026-11-26 10:00:00")
	settings := f.e.Settings()
	settings.EndDaysFromLast = 4
	if err := f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	// 构造已按月末倒数第 5 日建立的赛季窗口；设置变更本身不改已有本月窗口。
	if err := f.e.transaction(func(m *mutation) error {
		m.meta.Season.EndsAt = SeasonFor(f.now, 1, 4).EndsAt
		m.changed = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	f.report(1000, false)
	a := f.account("月赛结算账户")
	f.order(a, "buy", 1, 100000, "season-buy-one")
	cash := int64(0)
	if err := f.e.UpdateAccount(a.ID, &cash, nil); err != nil {
		t.Fatal(err)
	}
	f.report(0, true)
	f.atShanghai("2026-11-27 00:00:00")
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if !f.e.meta.Season.Settled || len(f.e.archives) != 1 || f.e.accounts[a.ID].Positions[1].Quantity != 0 {
		t.Fatal("season did not settle and archive")
	}
	restarted, err := NewEngine(f.store, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.archives) != 1 || !restarted.meta.Season.Settled {
		t.Fatal("archive/settled state not recovered")
	}
	f.e = restarted
	f.atShanghai("2026-12-01 00:00:00")
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if f.e.meta.Season.ID != "2026-12" || f.e.accounts[a.ID].Cash != f.e.Settings().InitialCash || len(f.e.accounts[a.ID].Positions) != 0 || f.e.stocks[1].Outstanding != 0 {
		t.Fatal("new month did not reset game ledger")
	}
	if f.e.accounts[a.ID].Username != a.Username {
		t.Fatal("season reset changed identity")
	}
}
func TestSubsecondInterestAndFeeSnapshotAcrossAdministration(t *testing.T) {
	f := newFixture(t)
	issuer := f.account("毫秒发行账户")
	a := f.account("毫秒借券账户")
	f.bind(issuer)
	settings := f.e.Settings()
	settings.BorrowAnnualPPM = 1000000
	if err := f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	f.order(a, "short", 1000, 100000, "subsecond-short")
	before := f.e.accounts[a.ID].Cash
	f.now = f.now.Add(800 * time.Millisecond)
	settings.BorrowAnnualPPM = 0
	if err := f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if before-f.e.accounts[a.ID].Cash != 2 {
		t.Fatal("old subsecond interest was dropped or charged at new rate")
	}
	b := f.account("手续费快照账户")
	p, err := f.e.Preview(b.ID, OrderInput{Stock: "1", Side: "buy", Quantity: 1, Price: 100000, ClientID: "snapshot-preview"})
	if err != nil {
		t.Fatal(err)
	}
	settings = f.e.Settings()
	settings.MinCommission = 1000000
	settings.PremiumPPM = 100000
	settings.TaxBrackets = []TaxBracket{{0, 320000}}
	if err = f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	o, err := f.e.Confirm(b.ID, p.PreviewID)
	if err != nil {
		t.Fatal(err)
	}
	if o.Commission != 0 || o.Tax != 0 || o.AveragePrice != 100000 {
		t.Fatal("admin change altered approved fee/tax/premium snapshot", o)
	}
	_, err = f.e.Preview(b.ID, OrderInput{Stock: "1", Side: "buy", Quantity: 1, Price: 100000, Leverage: 2, ClientID: "snapshot-preview"})
	mustCode(t, err, "IDEMPOTENCY_CONFLICT")
}
func TestMarketOpeningBoundaryKeepsOldQueuePriorityAndSettingsReschedule(t *testing.T) {
	f := newFixture(t)
	f.atShanghai("2026-11-09 10:00:00")
	f.profile("a-share")
	f.report(1000, false)
	oldAccount := f.account("开盘旧单账户")
	newAccount := f.account("开盘新单账户")
	stock := *f.e.stocks[1]
	stock.HourlyVolume = 100
	if err := f.e.UpdateStock(1, stock); err != nil {
		t.Fatal(err)
	}
	old := f.order(oldAccount, "buy", 200, 200000, "opening-old-order")
	f.atShanghai("2026-11-10 09:00:00")
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if f.e.orders[old.ID].EligibleHour > f.now.Unix()/3600 {
		t.Fatal("closed market postponed eligible order")
	}
	f.atShanghai("2026-11-10 09:30:00")
	f.report(1000, false)
	fresh := f.order(newAccount, "buy", 100, 100000, "opening-new-order")
	if fresh.Filled != 0 || f.e.orders[old.ID].Filled != 200 {
		t.Fatal("new opening order jumped queue")
	}
	f.atShanghai("2026-11-11 09:00:00")
	f.profile("meow")
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(5 * time.Minute)
	f.profile("a-share")
	if f.e.nextTick != 0 {
		t.Fatal("settings change did not reschedule ticker")
	}
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	want := f.now.Add(25 * time.Minute).UnixMilli()
	if f.e.nextTick != want {
		t.Fatalf("next session boundary %v want %v", time.UnixMilli(f.e.nextTick), time.UnixMilli(want))
	}
}
