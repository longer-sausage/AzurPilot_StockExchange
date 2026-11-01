package exchange

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu       sync.Mutex
	entities map[string]Entity
	history  map[int64]map[int64]int64
	fail     bool
	writes   int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{entities: map[string]Entity{}, history: map[int64]map[int64]int64{}}
}
func (s *memoryStore) Load() ([]Entity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Entity{}
	for _, v := range s.entities {
		x := v
		x.Data = append([]byte{}, v.Data...)
		out = append(out, x)
	}
	return out, nil
}
func (s *memoryStore) Commit(v []Entity, h []HistoryWrite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("injected commit failure")
	}
	s.writes++
	for _, x := range v {
		key := x.Kind + ":" + x.ID
		if x.Delete {
			delete(s.entities, key)
		} else {
			x.Data = append([]byte{}, x.Data...)
			s.entities[key] = x
		}
	}
	for _, x := range h {
		if s.history[x.StockID] == nil {
			s.history[x.StockID] = map[int64]int64{}
		}
		for _, p := range x.Points {
			s.history[x.StockID][p.T] = p.P
		}
	}
	return nil
}
func (s *memoryStore) Chart(id, from, to, interval int64) ([]Candle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	times := []int64{}
	for t := range s.history[id] {
		if t >= from && t <= to {
			times = append(times, t)
		}
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	out := []Candle{}
	for _, t := range times {
		p := s.history[id][t] * 100
		offset := int64(0)
		if interval >= 86400000 {
			offset = 28800000
		}
		bucket := (t+offset)/interval*interval - offset
		if len(out) == 0 || out[len(out)-1].T != bucket {
			out = append(out, Candle{bucket, p, p, p, p, 1})
		} else {
			c := &out[len(out)-1]
			c.H = max(c.H, p)
			c.L = min(c.L, p)
			c.C = p
			c.N++
		}
	}
	return out, nil
}
func (s *memoryStore) Manifest(id int64) ([]Month, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	times := []int64{}
	for t := range s.history[id] {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	buckets := map[string][]int64{}
	months := []string{}
	for _, t := range times {
		month := time.UnixMilli(t).In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01")
		if _, ok := buckets[month]; !ok {
			months = append(months, month)
		}
		buckets[month] = append(buckets[month], t)
	}
	out := []Month{}
	for _, month := range months {
		h := sha256.New()
		for _, t := range buckets[month] {
			fmt.Fprintf(h, "%d:%d\n", t, s.history[id][t])
		}
		out = append(out, Month{month, int64(len(buckets[month])), hex.EncodeToString(h.Sum(nil))})
	}
	return out, nil
}
func (s *memoryStore) Close() error { return nil }

type fixture struct {
	t        *testing.T
	e        *Engine
	store    *memoryStore
	now      time.Time
	instance InstanceRequest
	private  ed25519.PrivateKey
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, store: newMemoryStore(), now: time.Date(2026, 11, 1, 2, 10, 0, 0, time.UTC)}
	e, err := NewEngine(f.store, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	f.e = e
	s := DefaultSettings()
	s.StartDay = 1
	s.EndDaysFromLast = 0
	s.DelistThreshold = 0
	s.CommissionPPM = 0
	s.MinCommission = 0
	s.BorrowAnnualPPM = 0
	s.PremiumPPM = 0
	if err = e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	f.private = priv
	f.instance = InstanceRequest{InstanceID: "12345678-1234-1234-1234-123456789012", PublicKey: base64.StdEncoding.EncodeToString(pub), Username: "测试股票"}
	if _, err = e.InstanceRegister(f.sign("register", f.instance)); err != nil {
		t.Fatal(err)
	}
	f.report(1000, false)
	return f
}
func (f *fixture) sign(action string, r InstanceRequest) InstanceRequest {
	r.IssuedAt = f.now.Unix()
	r.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(f.private, []byte(instanceCanonical(action, r))))
	return r
}
func (f *fixture) report(points int64, backfill bool) {
	f.t.Helper()
	r := f.instance
	r.Username = ""
	r.Points = []Point{{f.now.UnixMilli(), points}}
	r.Backfill = backfill
	if _, err := f.e.InstanceReport(f.sign("report", r)); err != nil {
		f.t.Fatal(err)
	}
}
func (f *fixture) account(name string) *Account {
	f.t.Helper()
	result, err := f.e.BotRegister("app:"+name, name, "register-"+name, strings.Repeat("s", 64))
	if err != nil {
		f.t.Fatal(err)
	}
	a := f.e.accounts[result["accountId"].(int64)]
	password := result["password"].(string)
	if _, err = f.e.ChangePassword(a.ID, password, "NewStrong7!Password"); err != nil {
		f.t.Fatal(err)
	}
	// 每个交易账户都完成独立实例股票绑定，交易测试只操作目标股票 1。
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	req := InstanceRequest{InstanceID: fmt.Sprintf("87654321-4321-4321-4321-%012d", a.ID), PublicKey: base64.StdEncoding.EncodeToString(pub), Username: "归属" + name, IssuedAt: f.now.Unix()}
	req.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(instanceCanonical("register", req))))
	out, err := f.e.InstanceRegister(req)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = f.e.Bind(a.ID, out["bindingCode"].(string)); err != nil {
		f.t.Fatal(err)
	}
	return f.e.accounts[a.ID]
}
func (f *fixture) bind(a *Account) {
	f.t.Helper()
	if own := f.e.accounts[a.ID].BoundStockID; own != 0 && own != 1 {
		if err := f.e.DeleteStock(own); err != nil {
			f.t.Fatal(err)
		}
	}
	if _, err := f.e.Bind(a.ID, f.e.stocks[1].BindingCode); err != nil {
		f.t.Fatal(err)
	}
}
func (f *fixture) order(a *Account, side string, quantity, price int64, client string) *Order {
	f.t.Helper()
	p, err := f.e.Preview(a.ID, OrderInput{Stock: "MM000001", Side: side, Quantity: quantity, Price: price, ClientID: client})
	if err != nil {
		f.t.Fatal(err)
	}
	o, err := f.e.Confirm(a.ID, p.PreviewID)
	if err != nil {
		f.t.Fatal(err)
	}
	return o
}
func mustCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil || publicError(err).Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func TestIssuanceHourlyAuctionPremiumTaxAndIdempotency(t *testing.T) {
	f := newFixture(t)
	a := f.account("买方账户")
	s := f.e.Settings()
	s.HourlyVolume = 10
	s.IssuedShares = 15
	s.PremiumPPM = 100000
	s.TaxBrackets = []TaxBracket{{0, 80000}}
	if err := f.e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	stock := *f.e.stocks[1]
	stock.HourlyVolume = 10
	stock.IssuedShares = 15
	if err := f.e.UpdateStock(1, stock); err != nil {
		t.Fatal(err)
	}
	o := f.order(a, "buy", 12, 120000, "buy-first-12")
	if o.Filled != 10 || o.Remaining != 2 || o.Status != "pending" || o.AveragePrice != 105000 || o.Tax != 84000 {
		t.Fatalf("unexpected initial fill %+v", o)
	}
	cash := f.e.accounts[a.ID].Cash
	duplicate, err := f.e.Confirm(a.ID, o.PreviewID)
	if err != nil || duplicate.ID != o.ID || f.e.accounts[a.ID].Cash != cash {
		t.Fatal("duplicate confirm charged again", err)
	}
	p, err := f.e.Preview(a.ID, OrderInput{Stock: "1", Side: "buy", Quantity: 12, Price: 120000, ClientID: "buy-first-12"})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err = f.e.Confirm(a.ID, p.PreviewID)
	if err != nil || duplicate.ID != o.ID {
		t.Fatal("client id retry duplicated", err)
	}
	high := f.order(a, "buy", 10, 130000, "buy-high-10")
	f.now = f.now.Add(time.Hour)
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	if f.e.orders[high.ID].Filled != 5 || f.e.orders[o.ID].Filled != 10 || f.e.stocks[1].Outstanding != 15 {
		t.Fatal("higher price did not consume remaining inventory first")
	}
	if _, err = f.e.Cancel(a.ID, o.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.Cancel(a.ID, high.ID); err != nil {
		t.Fatal(err)
	}
	if f.e.accounts[a.ID].Reserved != 0 {
		t.Fatal("cancel did not release reservations")
	}
	bad := *f.e.stocks[1]
	bad.IssuedShares = 14
	mustCode(t, f.e.UpdateStock(1, bad), "STOCK_IN_USE")
}
func TestTaxBracketBoundaries(t *testing.T) {
	s := DefaultSettings()
	for _, tc := range []struct{ assets, want int64 }{{0, 0}, {3_000_000_000, 0}, {3_000_000_001, 80000}, {8_000_000_000, 80000}, {8_000_000_001, 160000}, {10_000_000_000, 160000}, {10_000_000_001, 320000}} {
		if got := taxRate(s, tc.assets); got != tc.want {
			t.Fatalf("assets=%d got %d want %d", tc.assets, got, tc.want)
		}
	}
	if rate(1, 80000) != 1 {
		t.Fatal("tax must round upward to a cent")
	}
}
func TestShortBindingProfitLossAndIndependentLimits(t *testing.T) {
	f := newFixture(t)
	issuer := f.account("发行者")
	trader := f.account("卖空者")
	_, err := f.e.Preview(trader.ID, OrderInput{Stock: "1", Side: "short", Quantity: 1, Price: 100000, ClientID: "short-unbound"})
	mustCode(t, err, "SHORT_UNBOUND")
	f.bind(issuer)
	_, err = f.e.Preview(issuer.ID, OrderInput{Stock: "1", Side: "buy", Quantity: 1, Price: 100000, ClientID: "self-trading"})
	mustCode(t, err, "SELF_TRADE")
	initialIssuer := f.e.accounts[issuer.ID].Cash
	o := f.order(trader, "short", 10, 100000, "short-open-10")
	if o.Filled != 10 || f.e.stocks[1].NormalUsed != 0 || f.e.stocks[1].BorrowUsed != 10 || f.e.accounts[issuer.ID].Cash != initialIssuer {
		t.Fatal("short incorrectly transferred money or volume")
	}
	f.now = f.now.Add(time.Minute)
	f.report(1100, false)
	f.order(trader, "cover", 5, 110000, "cover-loss-5")
	if got := f.e.accounts[issuer.ID].Cash - initialIssuer; got != 50000 {
		t.Fatalf("issuer loss credit %d want 50000", got)
	}
	f.now = f.now.Add(time.Minute)
	f.report(900, false)
	f.order(trader, "cover", 5, 90000, "cover-profit-5")
	if f.e.accounts[issuer.ID].Cash != initialIssuer+50000 || f.e.stocks[1].Borrowed != 0 {
		t.Fatal("profitable cover debited issuer or leaked borrowed shares")
	}
	if f.e.accounts[trader.ID].Cash != f.e.Settings().InitialCash {
		t.Fatalf("loss/profit did not cancel: %d", f.e.accounts[trader.ID].Cash)
	}
}
func TestFailedCommitRollsBackAllAffectedEntitiesAndNotifications(t *testing.T) {
	f := newFixture(t)
	issuer := f.account("原子发行者")
	trader := f.account("原子交易者")
	f.bind(issuer)
	f.order(trader, "short", 10, 100000, "atomic-short")
	f.now = f.now.Add(time.Minute)
	f.report(1100, false)
	p, err := f.e.Preview(trader.ID, OrderInput{Stock: "1", Side: "cover", Quantity: 10, Price: 110000, ClientID: "atomic-cover"})
	if err != nil {
		t.Fatal(err)
	}
	beforeAccounts := string(encode(f.e.accounts))
	beforeStock := string(encode(f.e.stocks))
	beforeOrders := string(encode(f.e.orders))
	version, ch := f.e.Changes()
	f.store.fail = true
	if _, err = f.e.Confirm(trader.ID, p.PreviewID); err == nil {
		t.Fatal("expected injected transaction failure")
	}
	if string(encode(f.e.accounts)) != beforeAccounts || string(encode(f.e.stocks)) != beforeStock || string(encode(f.e.orders)) != beforeOrders {
		t.Fatal("failed commit leaked cash/issuer/positions/inventory/order mutation")
	}
	version2, _ := f.e.Changes()
	if version != version2 {
		t.Fatal("version advanced on failed commit")
	}
	select {
	case <-ch:
		t.Fatal("notification fired before persistence")
	default:
	}
	f.store.fail = false
	o, err := f.e.Confirm(trader.ID, p.PreviewID)
	if err != nil || o.Filled != 10 {
		t.Fatal("retry did not execute", err)
	}
	restarted, err := NewEngine(f.store, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	if string(encode(restarted.accounts)) != string(encode(f.e.accounts)) || string(encode(restarted.orders)) != string(encode(f.e.orders)) {
		t.Fatal("restart lost committed ledger")
	}
	retry, err := restarted.Confirm(trader.ID, p.PreviewID)
	if err != nil || retry.ID != o.ID {
		t.Fatal("idempotency not recovered after restart", err)
	}
}
func TestHistoryCorrectionBackfillAndSignedIdentity(t *testing.T) {
	f := newFixture(t)
	r := f.instance
	r.Username = ""
	r.Points = []Point{{f.now.UnixMilli() - 3600000, 700}, {f.now.UnixMilli(), 1000}, {f.now.UnixMilli(), 1100}}
	r.Backfill = true
	if _, err := f.e.InstanceReport(f.sign("report", r)); err != nil {
		t.Fatal(err)
	}
	if f.e.stocks[1].Price != 110000 || len(f.store.history[1]) != 2 {
		t.Fatal("same millisecond correction not upserted")
	}
	r.Points = []Point{{f.now.UnixMilli() - 7200000, 600}}
	if _, err := f.e.InstanceReport(f.sign("report", r)); err != nil {
		t.Fatal(err)
	}
	if f.e.stocks[1].Price != 110000 {
		t.Fatal("historical backfill overwrote current quote")
	}
	candles, interval, err := f.e.Chart(1, f.now.UnixMilli()-9000000, f.now.UnixMilli(), 60)
	if err != nil || interval != 60 || len(candles) != 3 {
		t.Fatal(candles, interval, err)
	}
	manifest, err := f.e.InstanceManifest(f.sign("manifest", f.instance))
	if err != nil || len(manifest) != 1 || manifest[0].Count != 3 {
		t.Fatal(manifest, err)
	}
	tampered := f.sign("status", f.instance)
	tampered.InstanceID = "22345678-1234-1234-1234-123456789012"
	_, err = f.e.InstanceStatus(tampered)
	mustCode(t, err, "INVALID_SIGNATURE")
	changed := f.instance
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	changed.PublicKey = base64.StdEncoding.EncodeToString(pub)
	_, err = f.e.InstanceStatus(f.sign("status", changed))
	mustCode(t, err, "INVALID_SIGNATURE")
	if _, ok := publicStock(f.e.stocks[1])["bindingCode"]; ok {
		t.Fatal("public identity leaked")
	}
	before := len(f.store.history[1])
	f.store.fail = true
	r.Points = []Point{{f.now.UnixMilli() + 1000, 1200}}
	if _, err = f.e.InstanceReport(f.sign("report", r)); err == nil {
		t.Fatal("expected history persistence failure")
	}
	if len(f.store.history[1]) != before || f.e.stocks[1].Price != 110000 {
		t.Fatal("history failure changed current state")
	}
}
func TestForcedLiquidationProtectsIssuerAndBorrowAccrualRemainder(t *testing.T) {
	f := newFixture(t)
	issuer := f.account("强平发行者")
	trader := f.account("强平交易者")
	f.bind(issuer)
	f.order(trader, "short", 10, 100000, "liquidate-short")
	issuerCash := f.e.accounts[issuer.ID].Cash
	f.now = f.now.Add(time.Minute)
	f.report(4000, false)
	if f.e.accounts[trader.ID].Positions[1].ShortQuantity != 0 || f.e.stocks[1].Borrowed != 0 {
		t.Fatal("undercollateralized short not liquidated")
	}
	if f.e.accounts[issuer.ID].Cash <= issuerCash || f.e.accounts[trader.ID].Cash < 0 {
		t.Fatal("liquidation failed issuer protection")
	}
	f.now = f.now.Add(time.Minute)
	f.report(1000, false)
	f.order(trader, "short", 1, 100000, "interest-short")
	s := f.e.Settings()
	s.BorrowAnnualPPM = 1
	if err := f.e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	before := f.e.accounts[trader.ID].Cash
	f.now = f.now.Add(time.Second)
	if err := f.e.transaction(func(m *mutation) error { return m.accrue(m.account(trader.ID), 1, f.now.UnixMilli()) }); err != nil {
		t.Fatal(err)
	}
	if f.e.accounts[trader.ID].Cash != before || f.e.accounts[trader.ID].Positions[1].BorrowRemainder == 0 {
		t.Fatal("fractional borrow charge was lost")
	}
}
func TestSettingsDefaultFileAndNoIdleWrites(t *testing.T) {
	store := newMemoryStore()
	now := time.Now()
	e, err := NewEngine(store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err = e.InitializeDefaults("../../config/defaults.json"); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(e.Settings())
	if err != nil || !strings.Contains(string(raw), "\"issuedShares\":10000") {
		t.Fatal(err)
	}
	if err = e.Tick(); err != nil {
		t.Fatal(err)
	}
	writes := store.writes
	for range 100 {
		if err = e.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if store.writes != writes {
		t.Fatal("idle ticker wrote database")
	}
	s := e.Settings()
	s.PremiumPPM = 12345
	if err = e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewEngine(store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err = restarted.InitializeDefaults("missing-file"); err != nil || restarted.Settings().PremiumPPM != 12345 {
		t.Fatal("restart overwrote real config", err)
	}
}
