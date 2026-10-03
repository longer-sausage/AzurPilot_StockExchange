package exchange

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func historyIdentity(t *testing.T, f *fixture) (ed25519.PrivateKey, *InstanceBinding) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	r := HistoryReport{InstanceID: "12345678-1234-1234-1234-123456789012", PublicKey: base64.StdEncoding.EncodeToString(pub), Month: "2026-10", IssuedAt: f.now.Unix(), Points: []HistoryPoint{{f.now.UnixMilli(), 1000}}}
	r.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, []byte(historyCanonical(r))))
	binding, err := verifyHistory(r, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.BindInstance(f.owner.ID, binding); err != nil {
		t.Fatal(err)
	}
	return key, binding
}
func historySend(t *testing.T, f *fixture, b *InstanceBinding, points []HistoryPoint) {
	t.Helper()
	if err := f.e.IngestHistory(f.upload, HistoryReport{Month: "2026-10", Points: points}, b); err != nil {
		t.Fatal(err)
	}
}
func detailValue(t *testing.T, f *fixture, period, day string) StockDetail {
	t.Helper()
	raw, _, err := f.e.StockDetailJSON(f.owner.ID, period, "2026-10", day)
	if err != nil {
		t.Fatal(err)
	}
	var out StockDetail
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestWholeMonthBackfillCorrectionAndRestart(t *testing.T) {
	f := setup(t)
	_, binding := historyIdentity(t, f)
	start := time.Date(2026, 10, 1, 0, 0, 0, 123000000, time.UTC).UnixMilli()
	points := make([]HistoryPoint, 5001)
	for i := range points {
		points[i] = HistoryPoint{start + int64(i)*60000, int64(7000 + i%73)}
	}
	for i := 0; i < len(points); i += 1024 {
		historySend(t, f, binding, points[i:min(i+1024, len(points))])
	}
	historySend(t, f, binding, points[:1024])
	manifest, err := f.e.HistoryManifest(f.owner.ID, "2026-10")
	if err != nil || manifest.Count != 5001 {
		t.Fatalf("完整历史丢失/重复: %+v %v", manifest, err)
	}
	if err = f.e.IngestHistory(f.upload, HistoryReport{Month: "2026-10", Count: manifest.Count, Digest: manifest.Digest}, binding); err != nil {
		t.Fatal(err)
	}
	_, before, err := f.e.StockDetailJSON(f.owner.ID, "m5", "2026-10", "")
	if err != nil {
		t.Fatal(err)
	}
	corrected := points[17]
	corrected.ActionPoints = 999999 // 只检查格式，不做游戏真实性验证。
	historySend(t, f, binding, []HistoryPoint{corrected})
	after, etag, err := f.e.StockDetailJSON(f.owner.ID, "m5", "2026-10", "")
	if err != nil || before == etag {
		t.Fatalf("修正未使缓存失效: %v", err)
	}
	var detail StockDetail
	_ = json.Unmarshal(after, &detail)
	if detail.Coverage.Count != 5001 || detail.Coverage.ReconciledAt != 0 {
		t.Fatal("修正后仍显示旧校对")
	}
	f.e.Close()
	f.e, err = Open(f.path, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	next, err := f.e.HistoryManifest(f.owner.ID, "2026-10")
	if err != nil || next.Count != 5001 || next.Digest == manifest.Digest {
		t.Fatalf("重启/修正摘要错误: %+v %v", next, err)
	}
	// 摘要不匹配时整批回滚，不能破坏已经保存的历史和行情。
	bad := points[0]
	bad.ActionPoints = 123
	err = f.e.IngestHistory(f.upload, HistoryReport{Month: "2026-10", Points: []HistoryPoint{bad}, Count: next.Count, Digest: manifest.Digest}, binding)
	if publicError(err).Code != "HISTORY_MISMATCH" {
		t.Fatalf("应拒绝错误摘要: %v", err)
	}
	final, _ := f.e.HistoryManifest(f.owner.ID, "2026-10")
	if final.Digest != next.Digest {
		t.Fatal("失败校对没有回滚")
	}
}
func TestCandlesLateCorrectionAndRealTrades(t *testing.T) {
	f := setup(t)
	_, binding := historyIdentity(t, f)
	start := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC).UnixMilli()
	points := []HistoryPoint{{start + 1, 1000}, {start + 20000, 1600}, {start + 40000, 900}, {start + 59000, 1300}}
	historySend(t, f, binding, points)
	d := detailValue(t, f, "time", "2026-10-04")
	if len(d.Bars) != 1 {
		t.Fatalf("错误分钟数: %d", len(d.Bars))
	}
	b := d.Bars[0]
	if b.Open != 100000 || b.High != 160000 || b.Low != 90000 || b.Close != 130000 || b.Samples != 4 || b.Volume != 0 {
		t.Fatalf("OHLC 不正确: %+v", b)
	}
	points[1].ActionPoints = 1100
	historySend(t, f, binding, points[1:2])
	d = detailValue(t, f, "time", "2026-10-04")
	if d.Bars[0].High != 130000 {
		t.Fatal("迟到修正没有重算最高价")
	}
	for _, period := range []string{"day", "m5", "m10", "m20", "m30", "m60"} {
		d = detailValue(t, f, period, "")
		found := false
		for _, bar := range d.Bars {
			if bar.High == 130000 && bar.Low == 90000 {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s 聚合错误", period)
		}
	}
	input := order(f, "buy", "market", 10, 0, "real_trade_001")
	if _, err := f.e.Submit(f.trader.ID, input); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.Submit(f.trader.ID, input); err != nil {
		t.Fatal(err)
	} // 幂等请求不应重复计交易量。
	if _, err := f.e.Submit(f.trader.ID, order(f, "buy", "limit", 2, 50000, "real_pending_001")); err != nil {
		t.Fatal(err)
	}
	d = detailValue(t, f, "time", "2026-10-06")
	if len(d.Trades) != 1 || d.Trades[0].Quantity != 10 || len(d.Pending) != 1 || d.Summary.Volume != 10 || d.Summary.Turnover != 1000000 {
		t.Fatalf("公开成交/挂单统计错误: %+v", d)
	}
	f.now = f.now.Add(2 * time.Minute)
	if err := f.e.transaction(func() error { f.e.forcedClose(f.e.touch(f.trader.ID), f.now, "测试强平"); return nil }); err != nil {
		t.Fatal(err)
	}
	d = detailValue(t, f, "time", "2026-10-06")
	if len(d.Trades) != 2 || !d.Trades[0].Forced || d.Summary.Volume != 20 {
		t.Fatal("交易量未包含强制平仓")
	}
	var trades int
	_ = f.e.db.QueryRow("SELECT COUNT(*) FROM trades").Scan(&trades)
	historySend(t, f, binding, points)
	var after int
	_ = f.e.db.QueryRow("SELECT COUNT(*) FROM trades").Scan(&after)
	if trades != after {
		t.Fatal("补传重放了历史交易")
	}
}
func TestSignedHistoryHTTPAndConditionalDetail(t *testing.T) {
	f := setup(t)
	key, binding := historyIdentity(t, f)
	server := NewServer(f.e, Config{Mock: true, SessionSecret: "01234567890123456789012345678901"})
	report := HistoryReport{InstanceID: binding.InstanceID, PublicKey: binding.PublicKey, Month: "2026-10", IssuedAt: f.now.Unix(), Points: []HistoryPoint{{f.now.UnixMilli(), 999999}}}
	report.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, []byte(historyCanonical(report))))
	post := func(token, instance string, r HistoryReport) int {
		raw, _ := json.Marshal(map[string]any{"report": r})
		req := httptest.NewRequest("POST", "/api/quote-history", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-MMEX-Instance", instance)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		return w.Code
	}
	if status := post(f.upload, binding.Key, report); status != 200 {
		t.Fatalf("诚实玩家的数据应接受: %d", status)
	}
	if status := post(f.upload, "other", report); status != 401 {
		t.Fatalf("不匹配实例应拒绝: %d", status)
	}
	report.Points[0].ActionPoints++
	if status := post(f.upload, binding.Key, report); status != 400 {
		t.Fatal("损坏签名应拒绝")
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/stocks/%d?period=m20", f.owner.ID), nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("uploadToken")) || bytes.Contains(w.Body.Bytes(), []byte("clientId")) {
		t.Fatal("公开详情泄漏私有字段")
	}
	req.Header.Set("If-None-Match", "W/"+w.Header().Get("ETag"))
	second := httptest.NewRecorder()
	server.ServeHTTP(second, req)
	if second.Code != http.StatusNotModified || second.Body.Len() != 0 {
		t.Fatal("行情未复用条件缓存")
	}
}

func TestMillisecondLatestCannotBeOverwrittenByOlderCorrection(t *testing.T) {
	f := setup(t)
	_, binding := historyIdentity(t, f)
	stamp := f.now.UnixMilli()
	historySend(t, f, binding, []HistoryPoint{{stamp + 100, 1100}, {stamp + 700, 1500}})
	historySend(t, f, binding, []HistoryPoint{{stamp + 100, 900}})
	if f.e.players[f.owner.ID].Quote.Price != 150000 {
		t.Fatal("同一秒内较早记录的修正覆盖了最新报价")
	}
	historySend(t, f, binding, []HistoryPoint{{stamp + 700, 1400}})
	if f.e.players[f.owner.ID].Quote.Price != 140000 {
		t.Fatal("最新毫秒的修正未生效")
	}
	legacy, err := f.e.History(f.owner.ID)
	if err != nil || legacy[len(legacy)-1].Price != 140000 {
		t.Fatal("兼容报价接口没有显示修正后的价格")
	}
}
