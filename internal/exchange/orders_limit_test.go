package exchange

import (
	"fmt"
	"testing"
	"time"
)

func TestLimitOrdersFillPartialQuantityWithinPremium(t *testing.T) {
	for _, side := range []string{"buy", "sell", "short", "cover"} {
		for _, lot := range []int64{1, 3} {
			t.Run(fmt.Sprintf("%s/lot%d", side, lot), func(t *testing.T) {
				f := newFixture(t)
				a := f.account("限价交易者")
				if borrowing(side) {
					issuer := f.account("限价发行者")
					f.bind(issuer)
				}
				if side == "sell" {
					f.order(a, "buy", 9, 100000, "limit-seed-long")
				}
				if side == "cover" {
					f.order(a, "short", 9, 100000, "limit-seed-short")
				}
				settings := f.e.Settings()
				settings.PremiumPPM = 100000
				settings.ActiveMarket.LotSize = lot
				if err := f.e.SetSettings(settings); err != nil {
					t.Fatal(err)
				}
				stock := *f.e.stocks[1]
				stock.HourlyVolume, stock.BorrowHourlyVolume = 10, 20
				stock.NormalUsed, stock.BorrowUsed = 2, 4
				if err := f.e.UpdateStock(1, stock); err != nil {
					t.Fatal(err)
				}
				if err := f.e.transaction(func(m *mutation) error {
					s := m.stock(1)
					s.NormalUsed, s.BorrowUsed, s.Hour = stock.NormalUsed, stock.BorrowUsed, f.now.Unix()/3600
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				limit := int64(96000)
				if buying(side) {
					limit = 104000
				}
				want := int64(4)
				if borrowing(side) {
					want = 8
				}
				want = want / lot * lot
				o := f.order(a, side, 9, limit, "limit-partial-fill")
				if o.Filled != want || o.Remaining != 9-want || o.Status != "pending" {
					t.Fatalf("可成交部分被跳过：filled=%d remaining=%d status=%s，预期成交 %d", o.Filled, o.Remaining, o.Status, want)
				}
				price, _ := executionPrice(&stock, side, want, settings)
				if o.AveragePrice != price || (buying(side) && price > limit) || (!buying(side) && price < limit) {
					t.Fatalf("成交价错误或突破限价：got=%d want=%d limit=%d", o.AveragePrice, price, limit)
				}
				if borrowing(side) {
					if got := f.e.stocks[1]; got.NormalUsed != stock.NormalUsed || got.BorrowUsed != stock.BorrowUsed+want {
						t.Fatal("融券成交错误占用普通小时量")
					}
				} else if got := f.e.stocks[1]; got.NormalUsed != stock.NormalUsed+want || got.BorrowUsed != stock.BorrowUsed {
					t.Fatal("普通成交错误占用融券小时量")
				}
				cash, trades := f.e.accounts[a.ID].Cash, len(f.e.trades)
				duplicate, err := f.e.Confirm(a.ID, o.PreviewID)
				if err != nil || duplicate.ID != o.ID || duplicate.Filled != want || f.e.accounts[a.ID].Cash != cash || len(f.e.trades) != trades {
					t.Fatal("重复确认重复成交或扣款", err)
				}
				if _, err := f.e.Cancel(a.ID, o.ID); err != nil || f.e.accounts[a.ID].Reserved != 0 {
					t.Fatal("撤销余量未释放冻结", err)
				}
			})
		}
	}
}

func TestLimitOrderPartialFillRollbackRestartAndHourlyRemainder(t *testing.T) {
	f := newFixture(t)
	a := f.account("限价恢复交易者")
	settings := f.e.Settings()
	settings.PremiumPPM = 100000
	if err := f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	stock := *f.e.stocks[1]
	stock.HourlyVolume = 10
	if err := f.e.UpdateStock(1, stock); err != nil {
		t.Fatal(err)
	}
	p, err := f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "buy", Quantity: 10, Price: 102000, ClientID: "limit-rollback-restart"})
	if err != nil {
		t.Fatal(err)
	}
	before := string(encode([]any{f.e.meta, f.e.accounts, f.e.stocks, f.e.orders, f.e.trades}))
	_, changes := f.e.Changes()
	f.store.fail = true
	if _, err := f.e.Confirm(a.ID, p.PreviewID); err == nil {
		t.Fatal("未触发存储故障")
	}
	if string(encode([]any{f.e.meta, f.e.accounts, f.e.stocks, f.e.orders, f.e.trades})) != before {
		t.Fatal("失败事务泄漏资金、库存、委托或成交")
	}
	select {
	case <-changes:
		t.Fatal("失败事务发出变更通知")
	default:
	}
	f.store.fail = false
	o, err := f.e.Confirm(a.ID, p.PreviewID)
	if err != nil || o.Filled != 4 || o.Remaining != 6 || o.AveragePrice != 102000 || o.Reserved <= 0 {
		t.Fatal("重试未按限价成交可用部分", o, err)
	}
	restarted, err := NewEngine(f.store, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	f.e = restarted
	if got := f.e.orders[o.ID]; got.Filled != 4 || got.Remaining != 6 || len(f.e.trades) != 1 || f.e.accounts[a.ID].Reserved != got.Reserved {
		t.Fatal("重启丢失部分成交或冻结")
	}
	f.now = f.now.Add(time.Minute)
	f.report(1000, false)
	if f.e.orders[o.ID].Filled != 4 {
		t.Fatal("报价上报绕过整点撮合")
	}
	f.now = f.now.Add(time.Hour)
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if got := f.e.orders[o.ID]; got.Filled != 8 || got.Remaining != 2 || len(f.e.trades) != 2 {
		t.Fatal("整点未继续成交可用部分", got)
	}
	if err := f.e.Tick(); err != nil || len(f.e.trades) != 2 {
		t.Fatal("重复 Tick 重复成交", err)
	}
	f.now = f.now.Add(time.Hour)
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if got := f.e.orders[o.ID]; got.Status != "filled" || got.Filled != 10 || got.Reserved != 0 || f.e.accounts[a.ID].Reserved != 0 || len(f.e.trades) != 3 {
		t.Fatal("余量未全部成交或未释放冻结", got)
	}
}

func TestLimitOrderAtBaseQuoteWaitsWithoutCrossingPrice(t *testing.T) {
	for _, side := range []string{"buy", "sell", "short", "cover"} {
		t.Run(side, func(t *testing.T) {
			f := newFixture(t)
			a := f.account("限价边界交易者")
			if borrowing(side) {
				issuer := f.account("限价边界发行者")
				f.bind(issuer)
			}
			if side == "sell" {
				f.order(a, "buy", 10, 100000, "limit-boundary-long")
			}
			if side == "cover" {
				f.order(a, "short", 10, 100000, "limit-boundary-short")
			}
			settings := f.e.Settings()
			settings.PremiumPPM = 100000
			if err := f.e.SetSettings(settings); err != nil {
				t.Fatal(err)
			}
			cash, trades := f.e.accounts[a.ID].Cash, len(f.e.trades)
			o := f.order(a, side, 10, 100000, "limit-boundary-wait")
			if o.Filled != 0 || o.Remaining != 10 || o.Status != "pending" || f.e.accounts[a.ID].Cash != cash || len(f.e.trades) != trades {
				t.Fatal("基础价限价被含溢价成交突破", o)
			}
			f.now = f.now.Add(time.Hour)
			if err := f.e.Tick(); err != nil || f.e.orders[o.ID].Filled != 0 || len(f.e.trades) != trades {
				t.Fatal("整点恢复额度绕过限价", err)
			}
		})
	}
}
