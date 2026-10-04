package exchange

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestLeveragedBuyInterestRepaymentAndRestart(t *testing.T) {
	f := setup(t)
	in := order(f, "buy", "market", 30000, 0, "finance_0001")
	if _, err := f.e.Submit(f.trader.ID, in); err == nil {
		t.Fatal("现金不足的普通买入不应成交")
	}
	in.Leverage = 2
	o, err := f.e.Submit(f.trader.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := f.e.Account(f.trader.ID)
	if a.FinancingDebt != 1_500_000_000 || a.Equity != InitialCash-o.Fees.Total() || a.Available != InitialCash-1_500_000_000-o.Fees.Total() {
		t.Fatalf("融资买入账本错误: %+v", a)
	}
	f.now = f.now.Add(time.Hour)
	fee, _ := carriedInterest(1_500_000_000, 80000, 3600, 0)
	a, _ = f.e.Account(f.trader.ID)
	if a.FinancingAccrued != fee {
		t.Fatal("融资利息未计入实时净值")
	}
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if f.e.state.Season.Revenue.Financing != fee {
		t.Fatal("融资利息未计入交易所收入")
	}
	s := f.e.Settings()
	s.Active.CashDelayDays = 2
	if err = f.e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	partial, err := f.e.Submit(f.trader.ID, order(f, "sell", "market", 10000, 0, "finance_0002"))
	if err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	if a.FinancingDebt != 1_000_000_000 || a.Unsettled != 500_000_000-partial.Fees.Total() {
		t.Fatal("部分卖出未按比例还贷或交收重复占用已还本金")
	}
	before := a.Player.Cash
	if err = f.e.Close(); err != nil {
		t.Fatal(err)
	}
	f.e, err = Open(f.path, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	if a.FinancingDebt != 1_000_000_000 || a.Player.Cash != before || !f.e.marginOwners[f.trader.ID] {
		t.Fatal("融资持仓重启未恢复或重复计息")
	}
	last, err := f.e.Submit(f.trader.ID, order(f, "sell", "market", 20000, 0, "finance_0003"))
	if err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	if a.FinancingDebt != 0 || len(a.Player.Positions) != 0 || a.Equity != InitialCash-o.Fees.Total()-partial.Fees.Total()-last.Fees.Total()-fee {
		t.Fatal("平仓还款未闭合或重复扣本金")
	}
}

func TestLeveragedLongLiquidationAndSeasonReset(t *testing.T) {
	for _, price := range []int64{600, 100} {
		t.Run(symbol(price), func(t *testing.T) {
			f := setup(t)
			s := f.e.Settings()
			s.DelistThreshold = 0 // 此测试单独验证保证金强平，不触发行动力退市。
			s.Active.SellDelayDays = 1
			if err := f.e.SetSettings(s); err != nil {
				t.Fatal(err)
			}
			in := order(f, "buy", "market", 39900, 0, "long_risk_01")
			in.Leverage = 2
			if _, err := f.e.Submit(f.trader.ID, in); err != nil {
				t.Fatal(err)
			}
			f.now = f.now.Add(time.Minute)
			if err := f.e.Upload(f.upload, price, f.now.Unix()); err != nil {
				t.Fatal(err)
			}
			a, _ := f.e.Account(f.trader.ID)
			if len(a.Player.Positions) != 0 || a.FinancingDebt != 0 || a.Player.Orders[len(a.Player.Orders)-1].Reason != "维持保证金不足，系统强制平仓" {
				t.Fatal("融资多头应越过 T+1 限制强平")
			}
			if price == 100 && a.Equity >= 0 {
				t.Fatal("跳空形成的融资债务不应被抹除")
			}
			f.now = time.Unix(f.e.state.Season.EndsAt, 0)
			if err := f.e.Tick(); err != nil {
				t.Fatal(err)
			}
			results, err := f.e.Seasons()
			if err != nil || len(results) != 1 || results[0].Season.Revenue.Financing == 0 {
				t.Fatal("赛季归档应保留融资费用")
			}
			f.now = time.Date(2026, 11, 1, 1, 0, 0, 0, time.UTC)
			if err := f.e.Tick(); err != nil {
				t.Fatal(err)
			}
			a, _ = f.e.Account(f.trader.ID)
			if a.FinancingDebt != 0 || a.Equity != InitialCash || a.Player.Fees.Financing != 0 {
				t.Fatal("月赛重置融资账本失败")
			}
		})
	}
}

func TestLeveragedPendingCollateralAndRuleChanges(t *testing.T) {
	f := setup(t)
	in := order(f, "buy", "limit", 30000, 90000, "lever_limit1")
	in.Leverage = 3
	if _, err := f.e.Submit(f.trader.ID, in); err == nil {
		t.Fatal("超出 IMR 上限的杠杆不应受理")
	}
	in.Leverage = 2
	o, err := f.e.Submit(f.trader.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := f.e.Account(f.trader.ID)
	if a.FinancingDebt != 0 || a.Frozen != 1_350_000_000+FeesFor(o.Rules, "buy", 2_700_000_000).Total() {
		t.Fatal("挂单只能冻结自有资金，未成交不能放贷")
	}
	if err = f.e.Cancel(f.trader.ID, o.ID); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	if a.Frozen != 0 || a.Available != InitialCash {
		t.Fatal("融资挂单撤单未释放保证金")
	}
	in.ClientID = "lever_limit2"
	if _, err = f.e.Submit(f.trader.ID, in); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	if err = f.e.Upload(f.upload, 800, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	if a.FinancingDebt != 1_200_000_000 || a.Frozen != 0 {
		t.Fatal("触发融资买入应使用成交价而非限价放贷")
	}
	for _, side := range []string{"buy", "short", "sell", "cover"} {
		bad := order(f, side, "market", 1, 0, "bad_lever_"+side)
		bad.Leverage = 11
		if _, err = f.e.Submit(f.trader.ID, bad); err == nil {
			t.Fatal("非法杠杆没有在服务端拒绝")
		}
	}
	in.ClientID = "lever_limit3"
	in.Limit = 70000
	in.Quantity = 1
	if _, err = f.e.Submit(f.trader.ID, in); err != nil {
		t.Fatal(err)
	}
	s := f.e.Settings()
	s.Active.IMRPPM = 1_000_000
	if err = f.e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	if err = f.e.Upload(f.upload, 600, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	if a.Player.Orders[len(a.Player.Orders)-1].Status != "rejected" {
		t.Fatal("触发时应重新核验杠杆上限")
	}
}

func TestSelectedShortMarginAndFinancingRateChange(t *testing.T) {
	f := setup(t)
	in := order(f, "short", "market", 10000, 0, "short_lev_01")
	in.Leverage = 1
	if _, err := f.e.Submit(f.trader.ID, in); err != nil {
		t.Fatal(err)
	}
	a, _ := f.e.Account(f.trader.ID)
	if a.InitialMargin != 1_000_000_000 {
		t.Fatal("1 倍卖空应占用 100% 自有保证金")
	}
	in.ClientID = "short_lev_02"
	in.Leverage = 2
	if _, err := f.e.Submit(f.trader.ID, in); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	if a.InitialMargin != 1_500_000_000 {
		t.Fatal("不同杠杆追加空仓的保证金应加权合并")
	}
	if _, err := f.e.Submit(f.trader.ID, order(f, "cover", "market", 20000, 0, "short_lev_03")); err != nil {
		t.Fatal(err)
	}
	in = order(f, "buy", "market", 20000, 0, "rate_change1")
	in.Leverage = 2
	if _, err := f.e.Submit(f.trader.ID, in); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Hour)
	s := f.e.Settings()
	s.Active.FinancingAnnualPPM = 40000
	if err := f.e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Hour)
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	expected, _ := carriedInterest(1_000_000_000, 120000, 3600, 0)
	if a.Player.Fees.Financing != expected {
		t.Fatal("修改融资利率应先结算旧区间，再按新利率计提")
	}
}

func TestLegacyFinancingSettingsMigration(t *testing.T) {
	s := DefaultSettings()
	raw, _ := json.Marshal(State{Settings: s})
	legacy := strings.ReplaceAll(string(raw), `,"financingAnnualPPM":80000`, "")
	s.Active.FinancingAnnualPPM = 0
	for i := range s.Presets {
		s.Presets[i].FinancingAnnualPPM = 0
	}
	migrateFinancingRules(legacy, &s)
	if s.Active.FinancingAnnualPPM != 80000 || s.Presets[0].FinancingAnnualPPM != 80000 {
		t.Fatal("旧配置未补融资默认利率")
	}
	s.Active.FinancingAnnualPPM = 0
	raw, _ = json.Marshal(State{Settings: s})
	migrateFinancingRules(string(raw), &s)
	if s.Active.FinancingAnnualPPM != 0 {
		t.Fatal("显式零利率不应被迁移覆盖")
	}
}

func TestMixedCashAndLeveragedSharesUseExactCollateral(t *testing.T) {
	f := setup(t)
	if _, err := f.e.Submit(f.trader.ID, order(f, "buy", "market", 10, 0, "mixed_cash01")); err != nil {
		t.Fatal(err)
	}
	in := order(f, "buy", "market", 10000, 0, "mixed_cash02")
	in.Leverage = 2
	if _, err := f.e.Submit(f.trader.ID, in); err != nil {
		t.Fatal(err)
	}
	a, _ := f.e.Account(f.trader.ID)
	if a.Frozen != 0 || a.LongMargin != 501_000_000 {
		t.Fatal("混合持仓的平均率取整不应形成额外冻结")
	}
	if _, err := f.e.Submit(f.trader.ID, order(f, "sell", "market", 5005, 0, "mixed_cash03")); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	if a.Frozen != 0 || a.LongMargin != 250_500_000 || a.FinancingDebt != 250_000_000 {
		t.Fatal("部分平仓应保留精确的按股数加权保证金")
	}
}
