package exchange

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAuctionOldOrdersHavePriorityOverNewConfirmAtHourBoundary(t *testing.T) {
	f := newFixture(t)
	oldAccount := f.account("旧高价单账户")
	freshAccount := f.account("整点新单账户")
	stock := *f.e.stocks[1]
	stock.HourlyVolume = 1
	if err := f.e.UpdateStock(1, stock); err != nil {
		t.Fatal(err)
	}
	old := f.order(oldAccount, "buy", 2, 200000, "old-high-priority")
	if old.Filled != 1 {
		t.Fatal(old)
	}
	f.now = f.now.Add(time.Hour)
	fresh := f.order(freshAccount, "buy", 1, 100000, "fresh-low-priority")
	if fresh.Filled != 0 || f.e.orders[old.ID].Filled != 2 {
		t.Fatal("new confirmation stole earlier auction capacity", fresh, f.e.orders[old.ID])
	}
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if f.e.orders[fresh.ID].Filled != 0 {
		t.Fatal("ticker processed hour twice")
	}
}
func TestBindingRejectsExistingHoldingsAndPendingOrders(t *testing.T) {
	f := newFixture(t)
	a := f.account("绑定冲突账户")
	pending := f.order(a, "buy", 1, 90000, "bind-pending-order")
	if pending.Filled != 0 {
		t.Fatal(pending)
	}
	_, err := f.e.Bind(a.ID, f.e.stocks[1].BindingCode)
	mustCode(t, err, "ALREADY_BOUND")
	if _, err = f.e.Cancel(a.ID, pending.ID); err != nil {
		t.Fatal(err)
	}
	f.order(a, "buy", 1, 100000, "bind-existing-holding")
	if err = f.e.DeleteStock(f.e.accounts[a.ID].BoundStockID); err != nil {
		t.Fatal(err)
	}
	_, err = f.e.Bind(a.ID, f.e.stocks[1].BindingCode)
	mustCode(t, err, "BINDING_POSITION_CONFLICT")
}
func TestSmallSellWithoutFeeCashCannotPoisonHourlyAuction(t *testing.T) {
	f := newFixture(t)
	a := f.account("小额卖出账户")
	stock := *f.e.stocks[1]
	stock.HourlyVolume = 1
	if err := f.e.UpdateStock(1, stock); err != nil {
		t.Fatal(err)
	}
	f.order(a, "buy", 1, 100000, "small-buy-first")
	rules := f.e.Settings()
	rules.MinCommission = 500
	if err := f.e.SetSettings(rules); err != nil {
		t.Fatal(err)
	}
	pending := f.order(a, "sell", 1, 1, "small-pending-sell")
	cash := int64(0)
	if err := f.e.UpdateAccount(a.ID, &cash, nil); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	f.report(1, false)
	f.now = f.now.Add(time.Hour)
	if err := f.e.Tick(); err != nil {
		t.Fatal("unpayable sell poisoned auction", err)
	}
	if f.e.orders[pending.ID].Status != "rejected" {
		t.Fatal("unpayable order not cancelled")
	}
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
}
func TestBorrowAccrualSettlesOldPriceAndOldRateBeforeChange(t *testing.T) {
	f := newFixture(t)
	issuer := f.account("旧区间发行者")
	a := f.account("旧区间借券者")
	f.bind(issuer)
	rules := f.e.Settings()
	rules.BorrowAnnualPPM = 1000000
	if err := f.e.SetSettings(rules); err != nil {
		t.Fatal(err)
	}
	f.order(a, "short", 10, 100000, "old-price-borrow")
	cash := f.e.accounts[a.ID].Cash
	f.now = f.now.Add(24 * time.Hour)
	f.report(1100, false)
	want := int64(1_000_000 / 365)
	if got := cash - f.e.accounts[a.ID].Cash; got != want {
		t.Fatalf("old interval used new quote: got %d want %d", got, want)
	}
	before := f.e.accounts[a.ID].Cash
	remainder := f.e.accounts[a.ID].Positions[1].BorrowRemainder
	f.now = f.now.Add(24 * time.Hour)
	rules.BorrowAnnualPPM = 0
	if err := f.e.SetSettings(rules); err != nil {
		t.Fatal(err)
	}
	want, _ = carriedInterest(1_100_000, 1_000_000, 86400000, remainder)
	if got := before - f.e.accounts[a.ID].Cash; got != want {
		t.Fatalf("old interval used new fee: got %d want %d", got, want)
	}
}
func TestCoverEscrowRoundingUsesIdenticalReservationAndSettlement(t *testing.T) {
	f := newFixture(t)
	issuer := f.account("余数发行者")
	a := f.account("余数卖空者")
	f.bind(issuer)
	f.order(a, "short", 2, 100000, "rounding-short-two")
	f.now = f.now.Add(time.Minute)
	f.report(1002, false)
	f.order(a, "short", 1, 100200, "rounding-short-one")
	rules := f.e.Settings()
	rules.PremiumPPM = 100000
	rules.TaxBrackets = []TaxBracket{{RatePPM: 80000}}
	rules.ActiveMarket.MMRPPM = 50000
	if err := f.e.SetSettings(rules); err != nil {
		t.Fatal(err)
	}
	stock := *f.e.stocks[1]
	stock.BorrowHourlyVolume = 1
	if err := f.e.UpdateStock(1, stock); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	f.report(1850, false)
	pending := f.order(a, "cover", 1, 194250, "rounding-cover-one")
	reserved := f.e.accounts[a.ID].Reserved
	if reserved != 9658 {
		t.Fatalf("reserve must include both independent escrow remainders: %d", reserved)
	}
	if err := f.e.UpdateAccount(a.ID, &reserved, nil); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Hour)
	if err := f.e.Tick(); err != nil {
		t.Fatal("rounding poisoned auction", err)
	}
	if f.e.orders[pending.ID].Filled != 1 || f.e.accounts[a.ID].Cash != 0 {
		t.Fatal("cover rounding mismatch", f.e.accounts[a.ID].Cash)
	}
}
func TestConcurrentConfirmExactlyOnce(t *testing.T) {
	f := newFixture(t)
	a := f.account("并发委托账户")
	p, err := f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "buy", Quantity: 5, Price: 100000, ClientID: "parallel-confirm"})
	if err != nil {
		t.Fatal(err)
	}
	const workers = 12
	results := make(chan *Order, workers)
	failures := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o, err := f.e.Confirm(a.ID, p.PreviewID)
			if err != nil {
				failures <- err
			} else {
				results <- o
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var id int64
	for o := range results {
		if id != 0 && id != o.ID {
			t.Fatal("concurrent request produced duplicate orders")
		}
		id = o.ID
	}
	if f.e.accounts[a.ID].Positions[1].Quantity != 5 || f.e.stocks[1].NormalUsed != 5 {
		t.Fatal("concurrent confirm duplicated shares/volume")
	}
}
func TestMarketRulesPortfolioAndChartVersionsAreIndependent(t *testing.T) {
	f := newFixture(t)
	a := f.account("版本账户")
	marketBefore, marketVersion := f.e.Market()
	rulesBefore, rulesTag := f.e.RulesData()
	tagBefore, err := f.e.PortfolioTag(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.BotRegister("app:other-registration", "其他用户", "reg-other-user", strings.Repeat("s", 64)); err != nil {
		t.Fatal(err)
	}
	marketAfter, marketVersionAfter := f.e.Market()
	tagAfter, _ := f.e.PortfolioTag(a.ID)
	if string(marketBefore) != string(marketAfter) || marketVersion != marketVersionAfter || tagBefore != tagAfter {
		t.Fatal("unrelated registration invalidated market/portfolio")
	}
	rules := f.e.Settings()
	rules.PremiumPPM++
	if err = f.e.SetSettings(rules); err != nil {
		t.Fatal(err)
	}
	_, marketVersionAfter = f.e.Market()
	rulesAfter, newRulesTag := f.e.RulesData()
	if marketVersionAfter != marketVersion || newRulesTag == rulesTag || string(rulesAfter) == string(rulesBefore) {
		t.Fatal("rules and market versions not independent")
	}
	chartBefore, _ := f.e.ChartTag(1, 0, f.now.UnixMilli(), 60)
	f.report(1001, true)
	chartAfter, _ := f.e.ChartTag(1, 0, f.now.UnixMilli(), 60)
	if chartBefore == chartAfter {
		t.Fatal("same millisecond correction did not invalidate chart")
	}
}

func TestSeasonOpeningDelistsFreshPreseasonQuoteOnce(t *testing.T) {
	f := newFixture(t)
	f.atShanghai("2026-11-04 23:59:00")
	settings := f.e.Settings()
	settings.StartDay, settings.EndDaysFromLast, settings.DelistThreshold = 5, 4, 500
	if err := f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	f.report(100, false)
	if f.e.stocks[1].Delisted {
		t.Fatal("preseason quote delisted before opening")
	}
	f.atShanghai("2026-11-05 00:00:00")
	before := string(encode(f.e.stocks))
	f.store.fail = true
	if err := f.e.Tick(); err == nil {
		t.Fatal("expected opening persistence failure")
	}
	if f.e.meta.DelistCheckedStart != 0 || string(encode(f.e.stocks)) != before {
		t.Fatal("failed opening check leaked delisting or completion marker")
	}
	f.store.fail = false
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if !f.e.stocks[1].Delisted || f.e.meta.DelistCheckedStart != f.e.meta.Season.StartsAt {
		t.Fatal("fresh low quote was not delisted at season opening")
	}
	writes := f.store.writes
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if f.store.writes != writes {
		t.Fatal("opening check repeated on an idle tick")
	}
	restarted, err := NewEngine(f.store, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	if restarted.meta.DelistCheckedStart != f.e.meta.Season.StartsAt || !restarted.stocks[1].Delisted {
		t.Fatal("opening delisting marker did not survive restart")
	}
}

func TestSeasonOpeningIgnoresStaleQuoteAndLaterPureBackfill(t *testing.T) {
	f := newFixture(t)
	f.atShanghai("2026-11-03 23:59:00")
	settings := f.e.Settings()
	settings.StartDay, settings.EndDaysFromLast, settings.DelistThreshold = 5, 4, 500
	if err := f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	f.report(100, false)
	f.atShanghai("2026-11-05 00:00:00")
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if f.e.stocks[1].Delisted {
		t.Fatal("stale historical quote was treated as a valid opening quote")
	}
	f.report(1000, false)
	latestTime, latestPrice := f.e.stocks[1].ObservedAt, f.e.stocks[1].Price
	r := f.instance
	r.Username, r.Backfill = "", true
	r.Points = []Point{{T: latestTime - 60000, P: 50}}
	if _, err := f.e.InstanceReport(f.sign("report", r)); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Hour)
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if f.e.stocks[1].Delisted || f.e.stocks[1].ObservedAt != latestTime || f.e.stocks[1].Price != latestPrice {
		t.Fatal("pure historical backfill changed live quote or replayed delisting")
	}
}
