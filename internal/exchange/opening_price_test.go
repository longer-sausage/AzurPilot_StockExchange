package exchange

import (
	"encoding/json"
	"testing"
	"time"
)

func marketWithETag(t *testing.T, f *fixture) (Market, string) {
	t.Helper()
	raw, etag, err := f.e.MarketJSON()
	if err != nil {
		t.Fatal(err)
	}
	var market Market
	if err := json.Unmarshal(raw, &market); err != nil {
		t.Fatal(err)
	}
	return market, etag
}

func TestDailyOpeningPriceBackfillCorrectionAndRestart(t *testing.T) {
	f := setup(t)
	_, binding := historyIdentity(t, f)
	for _, ap := range []int64{800, 900} {
		f.now = f.now.Add(time.Minute)
		if err := f.e.Upload(f.upload, ap, f.now.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	m, etag := marketWithETag(t, f)
	stock := m.Stocks[0]
	if stock.Open != 100000 || stock.Quote.Price != 90000 || stock.Quote.Previous != 80000 || m.DelistThreshold != 500 {
		t.Fatalf("涨跌幅基准必须是开盘价而非前次报价: %+v", stock)
	}
	if detail := detailValue(t, f, "time", "2026-10-06"); detail.Stock.Open != stock.Open || detail.Summary.Open != stock.Open {
		t.Fatal("市场和证券详情开盘价不一致")
	}
	zone, _ := loadLocation("Asia/Shanghai")
	first := HistoryPoint{Time: time.Date(2026, 10, 6, 0, 0, 0, 123000000, zone).UnixMilli(), ActionPoints: 700}
	historySend(t, f, binding, []HistoryPoint{first})
	m, backfilled := marketWithETag(t, f)
	if etag == backfilled || m.Stocks[0].Open != 70000 || m.Stocks[0].Quote.Price != 90000 {
		t.Fatal("早期报价补传应修正开盘价并使市场缓存失效，不应回退最新报价")
	}
	first.ActionPoints = 750
	historySend(t, f, binding, []HistoryPoint{first})
	m, corrected := marketWithETag(t, f)
	if backfilled == corrected || m.Stocks[0].Open != 75000 || detailValue(t, f, "day", "2026-10-06").Stock.Open != 75000 {
		t.Fatal("同毫秒修正应更新市场及详情的开盘价")
	}
	f.e.Close()
	var err error
	f.e, err = Open(f.path, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	m, _ = marketWithETag(t, f)
	if m.Stocks[0].Open != 75000 {
		t.Fatal("重启后开盘价必须从持久历史恢复")
	}
}

func TestOpeningPriceResetsOnShanghaiMidnight(t *testing.T) {
	f := setupAt(t, time.Date(2026, 10, 6, 15, 59, 0, 0, time.UTC))
	_, binding := historyIdentity(t, f)
	m, before := marketWithETag(t, f)
	if m.Stocks[0].Open != 100000 {
		t.Fatal("午夜前应有当日开盘价")
	}
	f.now = f.now.Add(time.Minute) // UTC 16:00 为上海次日 00:00。
	m, after := marketWithETag(t, f)
	if before == after || m.Stocks[0].Open != 0 || detailValue(t, f, "time", "2026-10-06").Stock.Open != 0 {
		t.Fatal("跨日尚无报价时应清空当日开盘价，即使图表选择历史日期")
	}
	if err := f.e.Upload(f.upload, 1200, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	m, _ = marketWithETag(t, f)
	if m.Stocks[0].Open != 120000 || m.Stocks[0].Quote.Previous != 100000 {
		t.Fatal("次日第一条报价应成为新开盘价")
	}
	historySend(t, f, binding, []HistoryPoint{{f.now.Add(-2 * time.Minute).UnixMilli(), 800}})
	m, _ = marketWithETag(t, f)
	if m.Stocks[0].Open != 120000 {
		t.Fatal("前一天历史不能影响今日开盘价")
	}
}

func TestZeroOpeningPriceIsPreserved(t *testing.T) {
	f := setup(t)
	_, binding := historyIdentity(t, f)
	zone, _ := loadLocation("Asia/Shanghai")
	historySend(t, f, binding, []HistoryPoint{{time.Date(2026, 10, 6, 0, 0, 0, 1000000, zone).UnixMilli(), 0}})
	m, _ := marketWithETag(t, f)
	if m.Stocks[0].Open != 0 || m.Stocks[0].Quote.Price != 100000 || m.Stocks[0].Delisted {
		t.Fatal("真实零开盘价不能替换为后续非零报价，也不能重放历史退市")
	}
}
