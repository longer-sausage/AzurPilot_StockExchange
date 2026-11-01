package exchange

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func marketPreset(t *testing.T, id string) MarketRules {
	t.Helper()
	for _, rules := range DefaultMarketPresets() {
		if rules.ID == id {
			return rules
		}
	}
	t.Fatalf("missing market preset %s", id)
	return MarketRules{}
}

func marketTime(t *testing.T, value, zone string) time.Time {
	t.Helper()
	location, err := MarketLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, location)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestMarketPresetsRetainLegacyParametersAndJSON(t *testing.T) {
	type expected struct {
		id                                         string
		commission, minimum, stamp, rounding, levy int64
		imr, mmr, lot                              int64
		sellDays, cashDays                         int
		zone, stampSide                            string
		sessions                                   []string
		weekdays                                   bool
	}
	cases := []expected{
		{"meow", 300, 500, 0, 1, 0, 500000, 300000, 1, 0, 0, "Asia/Shanghai", "sell", []string{"00:00-24:00"}, false},
		{"a-share", 300, 500, 500, 1, 0, 1000000, 500000, 100, 1, 0, "Asia/Shanghai", "sell", []string{"09:30-11:30", "13:00-15:00"}, true},
		{"hk", 300, 3000, 1000, 100, 85, 500000, 300000, 100, 0, 2, "Asia/Shanghai", "both", []string{"09:30-12:00", "13:00-16:00"}, true},
		{"us", 0, 100, 0, 1, 0, 500000, 300000, 1, 0, 1, "America/New_York", "sell", []string{"09:30-16:00"}, true},
	}
	for _, want := range cases {
		t.Run(want.id, func(t *testing.T) {
			rules := marketPreset(t, want.id)
			if err := ValidateMarketRules(rules); err != nil {
				t.Fatal(err)
			}
			if rules.CommissionPPM != want.commission || rules.MinCommission != want.minimum || rules.StampPPM != want.stamp || rules.StampRound != want.rounding || rules.LevyPPM != want.levy || rules.IMRPPM != want.imr || rules.MMRPPM != want.mmr || rules.LotSize != want.lot || rules.SellDelayDays != want.sellDays || rules.CashDelayDays != want.cashDays || rules.Timezone != want.zone || rules.StampSide != want.stampSide || rules.WeekdaysOnly != want.weekdays || !reflect.DeepEqual(rules.Sessions, want.sessions) {
				t.Fatalf("legacy reference parameters changed: %+v", rules)
			}
			if rules.BorrowAnnualPPM != 80000 || rules.FinancingAnnualPPM != 80000 || rules.QuoteTTLSeconds != 86400 || len(rules.Holidays) != 0 {
				t.Fatal("legacy annual rates, quote TTL or holidays changed")
			}
			loaded, err := LoadMarketRules(filepath.Join("..", "..", "config", "presets", want.id+".json"))
			if err != nil || !reflect.DeepEqual(loaded, rules) {
				t.Fatalf("file and built-in preset differ: %v\nfile=%+v\nbuilt-in=%+v", err, loaded, rules)
			}
		})
	}
	loaded, err := LoadMarketPresets(filepath.Join("..", "..", "config", "presets"))
	if err != nil || len(loaded) != 4 {
		t.Fatalf("load preset directory: %d %v", len(loaded), err)
	}
}

func TestMarketFeesPreserveDirectionAndRounding(t *testing.T) {
	cases := []struct {
		preset, side string
		amount       int64
		want         Fees
	}{
		{"a-share", "buy", 1_000_000, Fees{Commission: 500}},
		{"a-share", "sell", 1_000_000, Fees{Commission: 500, Stamp: 500}},
		{"a-share", "short", 1_000_000, Fees{Commission: 500, Stamp: 500}},
		{"a-share", "cover", 1_000_000, Fees{Commission: 500}},
		{"hk", "buy", 1_234_567, Fees{Commission: 3000, Stamp: 1300, Levy: 105}},
		{"hk", "sell", 1_234_567, Fees{Commission: 3000, Stamp: 1300, Levy: 105}},
		{"hk", "buy", 1, Fees{Commission: 3000, Stamp: 100, Levy: 1}},
		{"us", "buy", 1_000_000, Fees{Commission: 100}},
		{"meow", "sell", 1_000_000, Fees{Commission: 500}},
	}
	for _, tc := range cases {
		t.Run(tc.preset+"-"+tc.side+"-"+idstr(tc.amount), func(t *testing.T) {
			if got := FeesFor(marketPreset(t, tc.preset), tc.side, tc.amount); got != tc.want {
				t.Fatalf("fees=%+v want %+v", got, tc.want)
			}
		})
	}
	fees := Fees{Commission: 1, Stamp: 2, Levy: 3, Borrow: 4, Financing: 5}
	fees.Add(Fees{Commission: 10, Stamp: 20, Levy: 30, Borrow: 40, Financing: 50})
	if fees.Total() != 165 {
		t.Fatal("fee snapshots lost a component", fees)
	}
}

func TestMarketSessionBoundariesHolidaysAndDST(t *testing.T) {
	a := marketPreset(t, "a-share")
	for _, tc := range []struct {
		clock string
		open  bool
	}{{"09:29:59", false}, {"09:30:00", true}, {"11:29:59", true}, {"11:30:00", false}, {"12:59:59", false}, {"13:00:00", true}, {"14:59:59", true}, {"15:00:00", false}} {
		if got := IsMarketOpen(marketTime(t, "2026-11-02 "+tc.clock, "Asia/Shanghai"), a); got != tc.open {
			t.Errorf("A-share %s open=%v", tc.clock, got)
		}
	}
	if IsMarketOpen(marketTime(t, "2026-11-07 10:00:00", "Asia/Shanghai"), a) {
		t.Fatal("weekend opened")
	}
	a.Holidays = []string{"2026-11-02"}
	if IsMarketOpen(marketTime(t, "2026-11-02 10:00:00", "Asia/Shanghai"), a) {
		t.Fatal("explicit holiday opened")
	}
	us := marketPreset(t, "us")
	for _, tc := range []struct {
		utc  string
		open bool
	}{{"2026-07-10 13:29:59", false}, {"2026-07-10 13:30:00", true}, {"2026-11-06 14:29:59", false}, {"2026-11-06 14:30:00", true}} {
		if got := IsMarketOpen(marketTime(t, tc.utc, "UTC"), us); got != tc.open {
			t.Errorf("New York DST %s open=%v", tc.utc, got)
		}
	}
	if MarketDay(marketTime(t, "2026-11-02 00:00:00", "UTC"), us) {
		t.Fatal("UTC Monday is still New York Sunday")
	}
	meow := marketPreset(t, "meow")
	if !IsMarketOpen(marketTime(t, "2026-11-08 23:59:59", "Asia/Shanghai"), meow) {
		t.Fatal("24-hour market closed on Sunday")
	}
}

func TestMarketSettlementUsesLocalBusinessDaysAndMilliseconds(t *testing.T) {
	a := marketPreset(t, "a-share")
	friday := marketTime(t, "2026-11-06 10:00:00", "Asia/Shanghai").Add(123 * time.Millisecond)
	if got := AvailableAt(friday, 0, a); got != friday.UnixMilli() {
		t.Fatal("T+0 dropped millisecond precision")
	}
	if got, want := AvailableAt(friday, 1, a), marketTime(t, "2026-11-09 00:00:00", "Asia/Shanghai").UnixMilli(); got != want {
		t.Fatalf("A-share T+1=%d want %d", got, want)
	}
	a.Holidays = []string{"2026-11-09"}
	if got, want := AvailableAt(friday, 1, a), marketTime(t, "2026-11-10 00:00:00", "Asia/Shanghai").UnixMilli(); got != want {
		t.Fatal("T+1 did not skip configured holiday")
	}
	hk := marketPreset(t, "hk")
	if got, want := AvailableAt(friday, hk.CashDelayDays, hk), marketTime(t, "2026-11-10 00:00:00", "Asia/Shanghai").UnixMilli(); got != want {
		t.Fatal("HK T+2 did not skip weekend")
	}
	us := marketPreset(t, "us")
	beforeDST := marketTime(t, "2026-03-06 10:00:00", "America/New_York")
	if got, want := AvailableAt(beforeDST, 1, us), marketTime(t, "2026-03-09 00:00:00", "America/New_York").UnixMilli(); got != want {
		t.Fatal("US T+1 did not honor DST boundary")
	}
}

func TestMarketSeasonShanghaiBoundaryAndMarginRounding(t *testing.T) {
	window := SeasonFor(marketTime(t, "2026-10-31 16:00:00", "UTC"), 5, 4)
	if window.ID != "2026-11" || window.StartsAt != marketTime(t, "2026-11-05 00:00:00", "Asia/Shanghai").UnixMilli() || window.EndsAt != marketTime(t, "2026-11-27 00:00:00", "Asia/Shanghai").UnixMilli() {
		t.Fatal("season did not use Shanghai milliseconds", window)
	}
	leap := SeasonFor(marketTime(t, "2028-02-01 00:00:00", "Asia/Shanghai"), 5, 4)
	if leap.EndsAt != marketTime(t, "2028-02-26 00:00:00", "Asia/Shanghai").UnixMilli() {
		t.Fatal("leap-month cutoff changed", leap)
	}
	us := marketPreset(t, "us")
	if InitialMargin(1_000_001, us) != 500001 || MaintenanceMargin(1_000_001, us) != 300001 {
		t.Fatal("margin must round upward in integer cents")
	}
}

func TestMarketRulesRejectInvalidProfiles(t *testing.T) {
	mutations := []func(*MarketRules){
		func(r *MarketRules) { r.ID = "bad id" },
		func(r *MarketRules) { r.CommissionPPM = -1 },
		func(r *MarketRules) { r.FinancingAnnualPPM = 1000001 },
		func(r *MarketRules) { r.MMRPPM = r.IMRPPM + 1 },
		func(r *MarketRules) { r.StampRound = 0 },
		func(r *MarketRules) { r.Timezone = "" },
		func(r *MarketRules) { r.Timezone = "Invalid/Zone" },
		func(r *MarketRules) { r.Source = "http://example.invalid" },
		func(r *MarketRules) { r.Sessions = []string{"09:30-12:00", "11:00-16:00"} },
		func(r *MarketRules) { r.Sessions = []string{"00:00-24:01"} },
		func(r *MarketRules) { r.Holidays = []string{"2026-02-30"} },
	}
	for i, mutate := range mutations {
		rules := DefaultMarketRules()
		mutate(&rules)
		if err := ValidateMarketRules(rules); err == nil {
			t.Errorf("invalid profile %d accepted", i)
		}
	}
	if err := ValidateMarketPresets([]MarketRules{DefaultMarketRules(), DefaultMarketRules()}); err == nil {
		t.Fatal("duplicate profile identity accepted")
	}
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte(`{"id":"meow","unexpected":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMarketRules(path); err == nil {
		t.Fatal("unknown JSON profile field accepted")
	}
}
