package exchange

import (
	"fmt"
	"testing"
	"time"
)

func TestMarketOrdersNeverRestRemainders(t *testing.T) {
	for _, kind := range []string{"market", "stop"} {
		for _, side := range []string{"buy", "sell", "short", "cover"} {
			for _, available := range []int64{0, 5, 9} {
				t.Run(fmt.Sprintf("%s/%s/available%d", kind, side, available), func(t *testing.T) {
					f := newFixture(t)
					a := f.account("市价交易者")
					if borrowing(side) {
						f.bind(f.account("市价发行者"))
					}
					if side == "sell" {
						f.order(a, "buy", 9, 100000, "market-seed-long")
					}
					if side == "cover" {
						f.order(a, "short", 9, 100000, "market-seed-short")
					}
					settings := f.e.Settings()
					settings.ActiveMarket.LotSize = 3
					if err := f.e.SetSettings(settings); err != nil {
						t.Fatal(err)
					}
					if err := f.e.transaction(func(m *mutation) error {
						s := m.stock(1)
						s.Hour = f.now.Unix() / 3600
						s.HourlyVolume, s.BorrowHourlyVolume = 10, 20
						s.NormalUsed, s.BorrowUsed = 0, 0
						if borrowing(side) {
							s.BorrowUsed = 20 - available
						} else {
							s.NormalUsed = 10 - available
						}
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					p, err := f.e.Preview(a.ID, OrderInput{Stock: "1", Side: side, Kind: kind, TimeInForce: "DAY", Quantity: 9, Price: 100000, ClientID: "market-no-rest"})
					if err != nil {
						t.Fatal(err)
					}
					cash, trades := f.e.accounts[a.ID].Cash, len(f.e.trades)
					o, err := f.e.Confirm(a.ID, p.PreviewID)
					if err != nil {
						t.Fatal(err)
					}
					want, status := available/3*3, "cancelled"
					if want == 9 {
						status = "filled"
					}
					if o.Filled != want || o.Remaining != 9-want || o.Status != status || o.Reserved != 0 || f.e.accounts[a.ID].Reserved != 0 || f.e.pending[o.ID] {
						t.Fatalf("市价余量进入挂单或未解冻：%+v", o)
					}
					if kind == "stop" && !o.Triggered {
						t.Fatal("无可用数量时遗漏止损触发")
					}
					if want == 0 && (f.e.accounts[a.ID].Cash != cash || len(f.e.trades) != trades || o.Tax+o.Commission+o.Stamp+o.Levy != 0) {
						t.Fatal("零成交仍扣款或记成交")
					}
					if want > 0 && (o.AveragePrice != 100000 || len(f.e.trades) != trades+1) {
						t.Fatal("成交价格或逐笔记录错误")
					}
					if s := f.e.stocks[1]; borrowing(side) && (s.BorrowUsed != 20-available+want || s.NormalUsed != 0) || !borrowing(side) && (s.NormalUsed != 10-available+want || s.BorrowUsed != 0) {
						t.Fatal("成交占用错误的小时额度")
					}
					cash, trades = f.e.accounts[a.ID].Cash, len(f.e.trades)
					duplicate, err := f.e.Confirm(a.ID, p.PreviewID)
					if err != nil || string(encode(duplicate)) != string(encode(o)) || f.e.accounts[a.ID].Cash != cash || len(f.e.trades) != trades {
						t.Fatal("重复确认再次成交或扣款", err)
					}
					f.now = f.now.Add(time.Hour)
					if err := f.e.Tick(); err != nil || f.e.orders[o.ID].Filled != want || len(f.e.trades) != trades {
						t.Fatal("市价余量在后续整点再次成交", err)
					}
				})
			}
		}
	}
}

func TestWaitingStopCancelsRemainderWhenTriggered(t *testing.T) {
	for _, inventory := range []int64{0, 3} {
		t.Run(fmt.Sprintf("inventory%d", inventory), func(t *testing.T) {
			f := newFixture(t)
			a := f.account("等待止损交易者")
			f.order(a, "buy", 1, 100000, "stop-existing-position")
			stock := *f.e.stocks[1]
			stock.IssuedShares = 1 + inventory
			if err := f.e.UpdateStock(1, stock); err != nil {
				t.Fatal(err)
			}
			p, err := f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "buy", Kind: "stop", Quantity: 9, Price: 110000, ClientID: "stop-wait-then-cancel"})
			if err != nil {
				t.Fatal(err)
			}
			o, err := f.e.Confirm(a.ID, p.PreviewID)
			if err != nil || o.Status != "pending" || o.Triggered || o.Reserved == 0 {
				t.Fatal("尚未触发的止损单未保留", o, err)
			}
			f.now = f.now.Add(time.Hour)
			f.report(1200, false)
			if err := f.e.Tick(); err != nil {
				t.Fatal(err)
			}
			if got := f.e.orders[o.ID]; !got.Triggered || got.Filled != inventory || got.Remaining != 9-inventory || got.Status != "cancelled" || got.Reserved != 0 || f.e.accounts[a.ID].Reserved != 0 {
				t.Fatal("止损触发后仍保留市价余量", got)
			}
		})
	}
}

func TestMarketRemainderRollbackRestartAndIdempotency(t *testing.T) {
	f := newFixture(t)
	a := f.account("市价恢复交易者")
	settings := f.e.Settings()
	settings.CommissionPPM, settings.MinCommission = 1000, 500
	settings.TaxBrackets = []TaxBracket{{0, 80000}}
	if err := f.e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	stock := *f.e.stocks[1]
	stock.HourlyVolume = 3
	if err := f.e.UpdateStock(1, stock); err != nil {
		t.Fatal(err)
	}
	p, err := f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "buy", Kind: "market", Quantity: 9, ClientID: "market-rollback-restart"})
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
		t.Fatal("失败事务泄漏资金、冻结、库存、委托或成交")
	}
	select {
	case <-changes:
		t.Fatal("失败事务发出变更通知")
	default:
	}
	f.store.fail = false
	cash := f.e.accounts[a.ID].Cash
	o, err := f.e.Confirm(a.ID, p.PreviewID)
	if err != nil || o.Status != "cancelled" || o.Filled != 3 || o.Remaining != 6 || o.Reserved != 0 {
		t.Fatal("重试未成交可用数量并撤销余量", o, err)
	}
	if o.Tax != 24000 || o.Commission != 500 || cash-f.e.accounts[a.ID].Cash != 324500 || f.e.accounts[a.ID].Reserved != 0 {
		t.Fatal("费用没有按实际成交部分收取或余量未解冻", o)
	}
	f.e, err = NewEngine(f.store, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	before = string(encode([]any{f.e.meta, f.e.accounts, f.e.stocks, f.e.orders, f.e.trades}))
	duplicate, err := f.e.Confirm(a.ID, p.PreviewID)
	if err != nil || string(encode(duplicate)) != string(encode(o)) || string(encode([]any{f.e.meta, f.e.accounts, f.e.stocks, f.e.orders, f.e.trades})) != before {
		t.Fatal("重启后重复确认改变账本", err)
	}
}

func TestLegacyRestingMarketOrdersAreCancelledAtomicallyOnStartup(t *testing.T) {
	f := newFixture(t)
	a := f.account("旧市价交易者")
	stock := *f.e.stocks[1]
	stock.HourlyVolume = 3
	if err := f.e.UpdateStock(1, stock); err != nil {
		t.Fatal(err)
	}
	market := f.order(a, "buy", 9, 100000, "legacy-market-remainder")
	triggered := f.order(a, "buy", 9, 100000, "legacy-triggered-remainder")
	limit := f.order(a, "buy", 1, 1, "legacy-limit-preserved")
	if err := f.e.transaction(func(m *mutation) error {
		m.order(market.ID).Kind = "market"
		o := m.order(triggered.ID)
		o.Kind, o.Triggered = "stop", true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p, err := f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "buy", Kind: "stop", Quantity: 1, Price: 110000, ClientID: "legacy-stop-preserved"})
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := f.e.Confirm(a.ID, p.PreviewID)
	if err != nil {
		t.Fatal(err)
	}
	// Confirm 会清理旧市价挂单；重新注入旧格式，覆盖仅重启的升级路径。
	if err := f.e.transaction(func(m *mutation) error {
		for _, id := range []int64{market.ID, triggered.ID} {
			o := m.order(id)
			o.Status, o.Reason, o.Reserved = "pending", "", 100000
			m.account(a.ID).Reserved += 100000 - f.e.orders[id].Reserved
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := string(encode(f.store.entities))
	f.store.fail = true
	if _, err := NewEngine(f.store, func() time.Time { return f.now }); err == nil {
		t.Fatal("启动清理在事务失败后仍成功")
	}
	if string(encode(f.store.entities)) != before {
		t.Fatal("失败的启动清理部分写入数据库")
	}
	f.store.fail = false
	f.e, err = NewEngine(f.store, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{market.ID, triggered.ID} {
		if o := f.e.orders[id]; o.Status != "cancelled" || o.Reserved != 0 || f.e.pending[id] {
			t.Fatal("旧市价挂单未撤销解冻", o)
		}
	}
	if got := f.e.orders[market.ID]; got.Filled != 3 || got.Remaining != 6 || len(f.e.trades) != 1 {
		t.Fatal("启动清理改变既有成交", got)
	}
	if f.e.orders[limit.ID].Status != "pending" || f.e.orders[waiting.ID].Status != "pending" || f.e.orders[waiting.ID].Triggered || f.e.accounts[a.ID].Reserved != f.e.orders[limit.ID].Reserved+f.e.orders[waiting.ID].Reserved {
		t.Fatal("启动清理影响限价或未触发止损委托")
	}
	writes := f.store.writes
	if _, err := NewEngine(f.store, func() time.Time { return f.now }); err != nil || f.store.writes != writes {
		t.Fatal("重复启动重复清理或写入", err)
	}
}
