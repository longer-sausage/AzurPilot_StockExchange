package exchange

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fixture struct {
	e             *Engine
	now           time.Time
	owner, trader *Player
	upload        string
	path          string
}

func setup(t *testing.T) *fixture {
	return setupAt(t, time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC))
}
func setupAt(t *testing.T, now time.Time) *fixture {
	t.Helper()
	f := &fixture{now: now, path: filepath.Join(t.TempDir(), "test.db")}
	var err error
	f.e, err = Open(f.path, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.e.Close() })
	f.owner, f.upload, err = f.e.Register("股东猫猫", "test-password-123", 1000, f.now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	f.trader, _, err = f.e.Register("交易猫猫", "test-password-123", 2000, f.now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func order(f *fixture, side, kind string, qty, price int64, cid string) OrderInput {
	return OrderInput{ClientID: cid, StockID: f.owner.ID, Side: side, Kind: kind, TIF: "GTC", Quantity: qty, Limit: price}
}
func TestRegistrationNormalizationAndPrivateHashes(t *testing.T) {
	f := setup(t)
	if f.trader.Cash != InitialCash {
		t.Fatal("初始资金错误")
	}
	if _, _, err := f.e.Register("股东猫猫", "test-password-123", 1, f.now.Unix()); err == nil {
		t.Fatal("重复名称应失败")
	}
	_, _, err := f.e.Register("ＡＢＣ猫", "test-password-123", 1, f.now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.e.Register("abc猫", "test-password-123", 1, f.now.Unix()); err == nil {
		t.Fatal("全角大小写应归一")
	}
	if _, err := f.e.Login("abc猫", "test-password-123"); err != nil {
		t.Fatal(err)
	}
	raw, _, err := f.e.MarketJSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Hash") || strings.Contains(string(raw), "password") {
		t.Fatal("公开行情泄漏凭据")
	}
}
func TestConcurrentOrdersIdempotencyAndRestart(t *testing.T) {
	f := setup(t)
	input := order(f, "buy", "market", 100, 0, "repeat_0001")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.e.Submit(f.trader.ID, input); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	a, _ := f.e.Account(f.trader.ID)
	if a.Player.Positions[f.owner.ID].Quantity != 100 {
		t.Fatal("重复成交")
	}
	expected := InitialCash - 100*100000 - FeesFor(f.e.Settings().Active, "buy", 100*100000).Total()
	if a.Player.Cash != expected {
		t.Fatalf("现金 %d，预期 %d", a.Player.Cash, expected)
	}
	f.e.Close()
	e, err := Open(f.path, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if _, err = e.Submit(f.trader.ID, input); err != nil {
		t.Fatal(err)
	}
	after, _ := e.Account(f.trader.ID)
	if after.Player.Cash != expected {
		t.Fatal("重启后重复扣款")
	}
}
func TestLimitFreezeCancelAndPriceTrigger(t *testing.T) {
	f := setup(t)
	o, err := f.e.Submit(f.trader.ID, order(f, "buy", "limit", 10, 90000, "limit_00001"))
	if err != nil || o.Status != "pending" {
		t.Fatal(err, o)
	}
	a, _ := f.e.Account(f.trader.ID)
	if a.Available >= a.Player.Cash {
		t.Fatal("挂单未冻结")
	}
	if err = f.e.Cancel(f.trader.ID, o.ID); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	if a.Available != InitialCash {
		t.Fatal("撤单未释放")
	}
	_, err = f.e.Submit(f.trader.ID, order(f, "buy", "limit", 10, 90000, "limit_00002"))
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	if err = f.e.Upload(f.upload, 800, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	if a.Player.Positions[f.owner.ID].Quantity != 10 || a.Player.Orders[len(a.Player.Orders)-1].Price != 80000 {
		t.Fatal("未按触发报价成交")
	}
}
func TestShortCollateralBorrowAndLiquidation(t *testing.T) {
	f := setup(t)
	_, err := f.e.Submit(f.trader.ID, order(f, "short", "market", 20000, 0, "short_00001"))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := f.e.Account(f.trader.ID)
	if a.ShortLiability != 2_000_000_000 || a.InitialMargin != 1_000_000_000 || a.Available > InitialCash-1_000_000_000 {
		t.Fatal("卖空所得未冻结")
	}
	f.now = f.now.Add(time.Hour)
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	expected := int64(2_000_000_000 * 80000 * 3600 / (365 * 86400 * 1_000_000))
	if a.Player.Fees.Borrow != expected {
		t.Fatalf("借券费错误 %d != %d", a.Player.Fees.Borrow, expected)
	}
	if err = f.e.Upload(f.upload, 5000, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	if len(a.Player.Positions) != 0 || a.Equity >= 0 {
		t.Fatal("应强平且保留跳空债务")
	}
	if a.Player.Orders[len(a.Player.Orders)-1].Reason != "维持保证金不足，系统强制平仓" {
		t.Fatal("强平原因丢失")
	}
}
func TestSettlementAndSameDaySale(t *testing.T) {
	f := setup(t)
	s := f.e.Settings()
	s.Active.LotSize = 100
	s.Active.SellDelayDays = 1
	s.Active.WeekdaysOnly = true
	s.Active.CashDelayDays = 2
	if err := f.e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.Submit(f.trader.ID, order(f, "buy", "market", 100, 0, "delay_00001")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.Submit(f.trader.ID, order(f, "sell", "market", 100, 0, "delay_00002")); err == nil {
		t.Fatal("T+1 当日卖出")
	}
	f.now = f.now.Add(24 * time.Hour)
	if err := f.e.Upload(f.upload, 1000, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.Submit(f.trader.ID, order(f, "sell", "market", 100, 0, "delay_00003")); err != nil {
		t.Fatal(err)
	}
	a, _ := f.e.Account(f.trader.ID)
	if a.Unsettled == 0 || a.Available == a.Player.Cash {
		t.Fatal("卖出资金应待交收")
	}
	f.now = f.now.Add(48 * time.Hour)
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	if a.Unsettled != 0 {
		t.Fatal("现金未交收")
	}
}
func TestCycleLeapMonthCutoffAndReset(t *testing.T) {
	f := setup(t)
	s := DefaultSettings()
	feb := seasonFor(time.Date(2028, 2, 10, 0, 0, 0, 0, time.UTC), s)
	if time.Unix(feb.EndsAt-1, 0).In(time.FixedZone("CST", 8*3600)).Day() != 25 {
		t.Fatal("闰月倒数第5日错误")
	}
	if _, err := f.e.Submit(f.trader.ID, order(f, "buy", "market", 10, 0, "season_0001")); err != nil {
		t.Fatal(err)
	}
	f.now = time.Unix(f.e.state.Season.EndsAt, 0)
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	a, _ := f.e.Account(f.trader.ID)
	if len(a.Player.Positions) != 0 || !f.e.state.Season.Settled {
		t.Fatal("赛季未结算")
	}
	results, err := f.e.Seasons()
	if err != nil || len(results) != 1 {
		t.Fatal("排名未归档", err)
	}
	equity := results[0].Rankings[1].Equity
	f.now = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	if a.Player.Cash != InitialCash || a.Player.Fees.Total() != 0 {
		t.Fatal("新赛季未重置")
	}
	after, _ := f.e.Seasons()
	if after[0].Rankings[1].Equity != equity {
		t.Fatal("归档排名改变")
	}
}
func TestSelfTradeStaleQuoteAndTampering(t *testing.T) {
	f := setup(t)
	in := order(f, "buy", "market", 10, 0, "self_000001")
	in.StockID = f.trader.ID
	if _, err := f.e.Submit(f.trader.ID, in); err == nil {
		t.Fatal("不得自我交易")
	}
	if err := f.e.Upload(f.upload, 999, f.now.Unix()-1); err == nil {
		t.Fatal("旧报价未拒绝")
	}
	if err := f.e.Upload(f.upload, 999, f.now.Unix()); err == nil {
		t.Fatal("同时间改价未拒绝")
	}
	f.now = f.now.Add(25 * time.Hour)
	if _, err := f.e.Submit(f.trader.ID, order(f, "buy", "market", 10, 0, "stale_00001")); err == nil {
		t.Fatal("过期报价仍成交")
	}
	newToken, err := f.e.RotateUpload(f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.e.Upload(f.upload, 1000, f.now.Unix()); err == nil {
		t.Fatal("旧凭据仍有效")
	}
	if err = f.e.Upload(newToken, 1000, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
}
func TestTradingDaysAndFees(t *testing.T) {
	r := DefaultSettings().Presets[2]
	f := FeesFor(r, "buy", 100001)
	if f.Stamp != 200 {
		t.Fatalf("港股印花税应整元向上取整，实得 %d", f.Stamp)
	}
	r.WeekdaysOnly = true
	r.Holidays = []string{"2026-10-05"}
	friday := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	next := time.Unix(availableAt(friday, 1, r), 0)
	if next.In(time.FixedZone("CST", 28800)).Day() != 6 {
		t.Fatal("未跳过周末及休市日")
	}
	us := DefaultSettings().Presets[3]
	if !sessionOpen(time.Date(2026, 10, 6, 14, 0, 0, 0, time.UTC), us) {
		t.Fatal("纽约夏令时间市场时段错误")
	}
	if !sessionOpen(time.Date(2026, 12, 8, 15, 0, 0, 0, time.UTC), us) {
		t.Fatal("纽约冬令时间市场时段错误")
	}
}
func TestCaptchaRequiredForRegisterAndLoginAndAdmin(t *testing.T) {
	f := setup(t)
	for _, mock := range []bool{false, true} {
		c := Config{Mock: mock, Domain: "stock.nanoda.work", SessionSecret: strings.Repeat("s", 32), AdminPassword: "test-admin-password", Origins: []string{"http://127.0.0.1:*"}}
		s := NewServer(f.e, c)
		for _, path := range []string{"/api/register", "/api/login", "/api/console/login"} {
			body := `{"username":"交易猫猫","password":"test-password-123","recaptchaToken":""}`
			if path == "/api/register" {
				body = `{"username":"新猫","password":"test-password-123","recaptchaToken":"","acceptedNotice":"2026-10-03"}`
			}
			if path == "/api/console/login" {
				body = `{"password":"test-admin-password","recaptchaToken":""}`
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
			if w.Code != 400 || !strings.Contains(w.Body.String(), `"code":"CAPTCHA_REQUIRED"`) {
				t.Fatalf("缺少验证码必须被验证码校验拒绝：mock=%v %s %d %s", mock, path, w.Code, w.Body.String())
			}
		}
		r := httptest.NewRequest("GET", "/api/console/settings", nil)
		r.Header.Set("Authorization", "Bearer "+s.sign(f.trader.ID, "player", time.Hour))
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("玩家获得管理员权限")
		}
	}
}
func TestRecaptchaDoesNotMatchHostnameOrAction(t *testing.T) {
	f := setup(t)
	s := NewServer(f.e, Config{SecretKey: "secret"})
	payload := map[string]any{"success": true, "hostname": "any-pilot.example", "action": "register"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("secret") != "secret" || r.Form.Get("response") != "token" {
			t.Error("未提交私密密钥与验证码 token")
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer ts.Close()
	s.SiteverifyURL = ts.URL
	if err := s.verify(t.Context(), "token"); err != nil {
		t.Fatal("停用域名验证后不得拒绝任意 hostname 或遗留 action", err)
	}
	delete(payload, "hostname")
	delete(payload, "action")
	if err := s.verify(t.Context(), "token"); err != nil {
		t.Fatal("未发送 action，仍应接受验证成功的响应", err)
	}
	payload["success"] = false
	requireCode(t, s.verify(t.Context(), "token"), "CAPTCHA_FAILED")
}

func TestConditionalMarketAndDiskFailure(t *testing.T) {
	f := setup(t)
	s := NewServer(f.e, Config{Origins: []string{}})
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/market", nil))
	etag := w.Header().Get("ETag")
	req := httptest.NewRequest("GET", "/api/market", nil)
	req.Header.Set("If-None-Match", etag)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 304 || w.Body.Len() != 0 {
		t.Fatal("共享行情缓存失效")
	}
	for _, validator := range []string{"W/" + etag, `"unrelated", W/` + etag} {
		req := httptest.NewRequest("GET", "/api/market", nil)
		req.Header.Set("If-None-Match", validator)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		if w.Code != 304 || w.Body.Len() != 0 {
			t.Fatal("压缩代理后的行情应继续命中 304 缓存")
		}
	}
	before, _ := f.e.Account(f.trader.ID)
	f.e.db.Close()
	_, err := f.e.Submit(f.trader.ID, order(f, "buy", "market", 10, 0, "disk_000001"))
	if err == nil {
		t.Fatal("磁盘失败仍成功")
	}
	after, _ := f.e.Account(f.trader.ID)
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if !bytes.Equal(a, b) {
		t.Fatal("落盘失败后内存账户改变")
	}
}

func TestDormantOrdersDoNotWriteOnTick(t *testing.T) {
	f := setup(t)
	_, err := f.e.Submit(f.trader.ID, order(f, "buy", "limit", 10, 100, "dormant_001"))
	if err != nil {
		t.Fatal(err)
	}
	revision := f.e.state.Revision
	f.now = f.now.Add(time.Minute)
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if f.e.state.Revision != revision {
		t.Fatal("未触发挂单不应写入 SQLite")
	}
}
func TestShortFeesSplitAtQuoteUpdate(t *testing.T) {
	f := setup(t)
	_, err := f.e.Submit(f.trader.ID, order(f, "short", "market", 100, 0, "split_00001"))
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Hour)
	if err = f.e.Upload(f.upload, 2000, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	a, _ := f.e.Account(f.trader.ID)
	first := a.Player.Fees.Borrow
	f.now = f.now.Add(time.Hour)
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	total := int64(100 * 100000 * 3 * 80000 * 3600 / (365 * 86400 * 1_000_000))
	if a.Player.Fees.Borrow != total || first <= 0 {
		t.Fatal("未按变价前后两个区间计借券费", a.Player.Fees.Borrow, total)
	}
}

func TestStopGapUsesExecutionPriceAndDayExpires(t *testing.T) {
	f := setup(t)
	_, err := f.e.Submit(f.trader.ID, order(f, "buy", "market", 100, 0, "stop_buy_01"))
	if err != nil {
		t.Fatal(err)
	}
	in := order(f, "sell", "stop", 100, 90000, "stop_sell01")
	o, err := f.e.Submit(f.trader.ID, in)
	if err != nil || o.Status != "pending" {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	if err = f.e.Upload(f.upload, 700, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	a, _ := f.e.Account(f.trader.ID)
	last := a.Player.Orders[len(a.Player.Orders)-1]
	if last.Price != 70000 || last.Status != "filled" || len(a.Player.Positions) != 0 {
		t.Fatal("止损跳空未按现价成交")
	}
	in = order(f, "buy", "limit", 10, 100, "day_order01")
	in.TIF = "DAY"
	o, err = f.e.Submit(f.trader.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(24 * time.Hour)
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	a, _ = f.e.Account(f.trader.ID)
	last = a.Player.Orders[len(a.Player.Orders)-1]
	if last.Status != "expired" || a.Frozen != 0 {
		t.Fatal("DAY 到期未释放资金")
	}
}
func TestLateJoinDoesNotChangeClosedSeasonRanking(t *testing.T) {
	f := setup(t)
	f.now = time.Unix(f.e.state.Season.EndsAt, 0).Add(time.Hour)
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	_, _, err := f.e.Register("迟到猫猫", "test-password-123", 2000, f.now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	f.e.mu.RLock()
	ranks := f.e.ranks(f.now.Unix())
	f.e.mu.RUnlock()
	if len(ranks) != 2 {
		t.Fatal("截止后新开户进入已结束赛季排名")
	}
}
func TestUnlimitedBorrowInventoryAndIntegerSafety(t *testing.T) {
	f := setup(t)
	s := f.e.Settings()
	s.DelistThreshold = 0 // 此测试仅验证低价下的融券数量与整数安全边界。
	if err := f.e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	if err := f.e.Upload(f.upload, 1, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := f.e.Submit(f.trader.ID, order(f, "short", "market", 1_000_000, 0, fmt.Sprintf("unlimited_%d", i))); err != nil {
			t.Fatal("融券不应限制为单笔数量上限", err)
		}
	}
	a, _ := f.e.Account(f.trader.ID)
	if a.Player.Positions[f.owner.ID].Quantity != -2_000_000 {
		t.Fatal("未允许累计卖空超过单笔上限")
	}
	// 模拟通过历史涨跌积累到极大持仓的账户，检查最坏未来价格也不会溢出。
	quantity := LedgerSafetyLimit / MaxQuotePrice
	if err := f.e.transaction(func() error {
		p := f.e.touch(f.trader.ID)
		p.Cash = InitialCash
		p.Positions[f.owner.ID] = &Position{StockID: f.owner.ID, Quantity: quantity, Cost: quantity * 100, Lots: []Lot{{quantity, f.now.Unix()}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := f.e.Account(f.trader.ID)
	if _, err := f.e.Submit(f.trader.ID, order(f, "buy", "market", 1, 0, "overflow_01")); err == nil || publicError(err).Code != "NUMERICAL_LIMIT" {
		t.Fatal("超出安全数值边界仍能开仓", err)
	}
	after, _ := f.e.Account(f.trader.ID)
	if after.Player.Cash != before.Player.Cash || after.Player.Positions[f.owner.ID].Quantity != quantity {
		t.Fatal("被拒委托改变了账本")
	}
	if _, err := f.e.Submit(f.trader.ID, order(f, "sell", "market", 1_000_000, 0, "safe_close_01")); err != nil {
		t.Fatal("安全边界不应阻止平仓", err)
	}
}
func BenchmarkMarketCache1000Players(b *testing.B) {
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	settings := DefaultSettings()
	e, err := Open(filepath.Join(b.TempDir(), "benchmark.db"), func() time.Time { return now })
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { e.Close() })
	e.state = State{Settings: settings, Season: seasonFor(now, settings), Revision: 1}
	for i := int64(1); i <= 1000; i++ {
		e.players[i] = &Player{ID: i, Username: fmt.Sprintf("玩家%d", i), Cash: InitialCash, Quote: Quote{Price: 100000, Previous: 100000, ObservedAt: now.Unix()}, Positions: map[int64]*Position{}, Orders: []*Order{}}
	}
	if _, _, err := e.MarketJSON(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := e.MarketJSON(); err != nil {
			b.Fatal(err)
		}
	}
}
