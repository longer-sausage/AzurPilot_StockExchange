package exchange

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"testing"
	"time"
)

func marketValue(t *testing.T, e *Engine) Market {
	t.Helper()
	raw, _, err := e.MarketJSON()
	if err != nil {
		t.Fatal(err)
	}
	var m Market
	if err = json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestConfigurableFundingKeepsCurrentBalancesAndReturns(t *testing.T) {
	f := setup(t)
	if _, err := f.e.Submit(f.trader.ID, order(f, "buy", "market", 10000, 0, "funding_buy_01")); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(16 * time.Second)
	if err := f.e.Upload(f.upload, 1500, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	before, _ := f.e.Account(f.trader.ID)
	s := f.e.Settings()
	s.InitialCash = 2_500_000_000
	if err := f.e.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	after, _ := f.e.Account(f.trader.ID)
	if before.Equity != after.Equity || before.Player.Cash != after.Player.Cash || after.Player.InitialCash != InitialCash {
		t.Fatal("配置不应改变已有账户的现金或起始本金")
	}
	p, _, err := f.e.Register("新本金猫猫", "test-password-123", 1000, f.now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if p.Cash != s.InitialCash || p.InitialCash != s.InitialCash {
		t.Fatal("新账户未领取配置的资金")
	}
	m := marketValue(t, f.e)
	if m.InitialCash != s.InitialCash {
		t.Fatal("公开行情未更新初始资金")
	}
	for _, rank := range m.Rankings {
		if rank.PlayerID == f.trader.ID && rank.ReturnPPM != 249850 {
			t.Fatal("老账户收益率应继续按 2 千万本金计算", rank)
		}
		if rank.PlayerID == p.ID && rank.ReturnPPM != 0 {
			t.Fatal("新账户初始收益率应为零", rank)
		}
	}
	server := NewServer(f.e, Config{})
	w := httptest.NewRecorder()
	server.ServeHTTP(w, httptest.NewRequest("GET", "/api/meta", nil))
	var meta struct {
		InitialCash int64 `json:"initialCash"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &meta); err != nil || meta.InitialCash != s.InitialCash {
		t.Fatal("开户元数据未更新资金", err)
	}
	f.e.Close()
	f.e, err = Open(f.path, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	old, _ := f.e.Account(f.trader.ID)
	newAccount, _ := f.e.Account(p.ID)
	if f.e.InitialFunding() != s.InitialCash || old.Player.InitialCash != InitialCash || newAccount.Player.InitialCash != s.InitialCash {
		t.Fatal("配置或个人本金未持久化")
	}
	f.now = time.Date(2026, 11, 1, 1, 0, 0, 0, time.UTC)
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{f.owner.ID, f.trader.ID, p.ID} {
		a, _ := f.e.Account(id)
		if a.Player.Cash != s.InitialCash || a.Player.InitialCash != s.InitialCash || len(a.Player.Positions) != 0 {
			t.Fatal("下一赛季未按配置重置资金")
		}
	}
	archives, err := f.e.Seasons()
	if err != nil || len(archives) != 1 {
		t.Fatal("旧赛季未归档", err)
	}
	for _, rank := range archives[0].Rankings {
		if rank.PlayerID == f.trader.ID && rank.ReturnPPM != returnPPM(rank.Equity, InitialCash) {
			t.Fatal("归档收益率使用了新本金")
		}
	}
}

func TestLegacyFundingMigrationAndValidation(t *testing.T) {
	f := setup(t)
	var raw string
	if err := f.e.db.QueryRow("SELECT data FROM metadata WHERE id=1").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var state map[string]json.RawMessage
	var settings map[string]json.RawMessage
	json.Unmarshal([]byte(raw), &state)
	json.Unmarshal(state["settings"], &settings)
	delete(settings, "initialCash")
	state["settings"], _ = json.Marshal(settings)
	data, _ := json.Marshal(state)
	if _, err := f.e.db.Exec("UPDATE metadata SET data=? WHERE id=1", string(data)); err != nil {
		t.Fatal(err)
	}
	for id, p := range f.e.players {
		data, _ = json.Marshal(storedPlayer{Player: p, Folded: p.Folded, PasswordHash: p.PasswordHash, UploadHash: p.UploadHash, SessionVersion: p.SessionVersion})
		var stored map[string]json.RawMessage
		var player map[string]json.RawMessage
		json.Unmarshal(data, &stored)
		json.Unmarshal(stored["player"], &player)
		delete(player, "initialCash")
		stored["player"], _ = json.Marshal(player)
		data, _ = json.Marshal(stored)
		if _, err := f.e.db.Exec("UPDATE players SET data=? WHERE id=?", string(data), id); err != nil {
			t.Fatal(err)
		}
	}
	f.e.Close()
	var err error
	f.e, err = Open(f.path, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	a, _ := f.e.Account(f.trader.ID)
	if f.e.InitialFunding() != InitialCash || a.Player.InitialCash != InitialCash || a.Player.Cash != InitialCash {
		t.Fatal("旧数据库应保留默认 2 千万本金")
	}
	for _, amount := range []int64{0, 99, MaxNotional + 1} {
		s := f.e.Settings()
		s.InitialCash = amount
		requireCode(t, f.e.SetSettings(s), "INVALID_RULES")
	}
	for _, amount := range []int64{100, InitialCash, MaxNotional} {
		s := f.e.Settings()
		s.InitialCash = amount
		if err := ValidateSettings(s); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFundingReturnArithmetic(t *testing.T) {
	for _, tc := range []struct{ equity, principal, want int64 }{
		{InitialCash, InitialCash, 0}, {0, InitialCash, -1_000_000}, {-InitialCash, InitialCash, -2_000_000},
		{LedgerSafetyLimit, 100, math.MaxInt64}, {-LedgerSafetyLimit, 100, -math.MaxInt64},
		{12_345, 10_000, 234_500},
	} {
		if got := returnPPM(tc.equity, tc.principal); got != tc.want {
			t.Fatalf("%+v: got %d", tc, got)
		}
	}
}
