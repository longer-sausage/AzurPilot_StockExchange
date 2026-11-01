package exchange

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MarketRules 保留原现实参考预设的完整参数；发行额度和新游戏税率由外层配置独立管理。
type MarketRules struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	CommissionPPM      int64    `json:"commissionPPM"`
	MinCommission      int64    `json:"minCommission"`
	StampPPM           int64    `json:"stampPPM"`
	StampSide          string   `json:"stampSide"`
	StampRound         int64    `json:"stampRound"`
	LevyPPM            int64    `json:"levyPPM"`
	BorrowAnnualPPM    int64    `json:"borrowAnnualPPM"`
	FinancingAnnualPPM int64    `json:"financingAnnualPPM"`
	IMRPPM             int64    `json:"imrPPM"`
	MMRPPM             int64    `json:"mmrPPM"`
	LotSize            int64    `json:"lotSize"`
	SellDelayDays      int      `json:"sellDelayDays"`
	CashDelayDays      int      `json:"cashDelayDays"`
	Timezone           string   `json:"timezone"`
	Sessions           []string `json:"sessions"`
	WeekdaysOnly       bool     `json:"weekdaysOnly"`
	Holidays           []string `json:"holidays"`
	QuoteTTLSeconds    int64    `json:"quoteTTLSeconds"`
	Source             string   `json:"source"`
}

// Fees 是落账及委托费用快照，金额均为整数分。
type Fees struct {
	Commission int64 `json:"commission"`
	Stamp      int64 `json:"stamp"`
	Levy       int64 `json:"levy"`
	Borrow     int64 `json:"borrow"`
	Financing  int64 `json:"financing"`
}

func (f Fees) Total() int64 { return f.Commission + f.Stamp + f.Levy + f.Borrow + f.Financing }
func (f *Fees) Add(g Fees) {
	f.Commission += g.Commission
	f.Stamp += g.Stamp
	f.Levy += g.Levy
	f.Borrow += g.Borrow
	f.Financing += g.Financing
}

func DefaultMarketRules() MarketRules {
	return MarketRules{ID: "meow", Name: "茗喵全天候", Description: "全天交易，T+0，无印花税；最高 2 倍杠杆、行动力定价，股数与交易量受本版额度约束。", CommissionPPM: 300, MinCommission: 500, StampSide: "sell", StampRound: 1, BorrowAnnualPPM: 80000, FinancingAnnualPPM: 80000, IMRPPM: 500000, MMRPPM: 300000, LotSize: 1, Timezone: "Asia/Shanghai", Sessions: []string{"00:00-24:00"}, Holidays: []string{}, QuoteTTLSeconds: 86400}
}

// DefaultMarketPresets 保留原参数，仅修正过时的无限额度描述；新税率不写入现实参考预设。
func DefaultMarketPresets() []MarketRules {
	base := DefaultMarketRules()
	a := base
	a.ID = "a-share"
	a.Name = "A 股参考"
	a.Description = "百股整手、T+1 可卖、工作日分段交易。发行、借券与小时成交量受游戏额度约束；佣金为可调示例。"
	a.StampPPM = 500
	a.LotSize = 100
	a.SellDelayDays = 1
	a.IMRPPM = 1000000
	a.MMRPPM = 500000
	a.WeekdaysOnly = true
	a.Sessions = []string{"09:30-11:30", "13:00-15:00"}
	a.Source = "https://www.sse.com.cn/lawandrules/sselawsrules2025/stocks/exchange/c/c_20260424_10816482.shtml"
	h := base
	h.ID = "hk"
	h.Name = "港股参考"
	h.Description = "双边印花税 0.1%，整元向上取整，T+2 现金交收；手数与佣金为统一游戏示例。"
	h.CommissionPPM = 300
	h.MinCommission = 3000
	h.StampPPM = 1000
	h.StampSide = "both"
	h.StampRound = 100
	h.LevyPPM = 85
	h.CashDelayDays = 2
	h.LotSize = 100
	h.WeekdaysOnly = true
	h.Sessions = []string{"09:30-12:00", "13:00-16:00"}
	h.Source = "https://www.hkex.com.hk/Services/Rules-and-Forms-and-Fees/Fees/Securities-(Hong-Kong)/Trading/Transaction?sc_lang=en"
	u := base
	u.ID = "us"
	u.Name = "美股参考"
	u.Description = "整股、T+1 现金交收，50% 初始 / 30% 维持做空保证金；佣金为游戏示例。"
	u.CommissionPPM = 0
	u.MinCommission = 100
	u.CashDelayDays = 1
	u.WeekdaysOnly = true
	u.Timezone = "America/New_York"
	u.Sessions = []string{"09:30-16:00"}
	u.Source = "https://www.finra.org/rules-guidance/notices/21-12"
	return []MarketRules{base, a, h, u}
}

var marketPresetID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
var marketLocations sync.Map

func MarketLocation(name string) (*time.Location, error) {
	if cached, ok := marketLocations.Load(name); ok {
		return cached.(*time.Location), nil
	}
	location, err := time.LoadLocation(name)
	if err == nil {
		marketLocations.Store(name, location)
	}
	return location, err
}

func ValidateMarketRules(r MarketRules) error {
	if !marketPresetID.MatchString(r.ID) || len([]rune(r.Name)) < 1 || len([]rune(r.Name)) > 40 || len(r.Description) > 1500 || len(r.Source) > 500 {
		return fail("INVALID_RULES", "预设名称或标识无效")
	}
	if r.Source != "" {
		u, err := url.Parse(r.Source)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return fail("INVALID_RULES", "参考来源须为 HTTPS URL")
		}
	}
	for _, value := range []int64{r.CommissionPPM, r.StampPPM, r.LevyPPM, r.BorrowAnnualPPM, r.FinancingAnnualPPM} {
		if value < 0 || value > 1_000_000 {
			return fail("INVALID_RULES", "费率须在 0% 到 100% 之间")
		}
	}
	if r.MinCommission < 0 || r.MinCommission > 1_000_000 || r.IMRPPM < 100_000 || r.IMRPPM > 2_000_000 || r.MMRPPM < 50_000 || r.MMRPPM > r.IMRPPM || r.LotSize < 1 || r.LotSize > 10000 || r.StampRound < 1 || r.StampRound > 10000 || r.SellDelayDays < 0 || r.SellDelayDays > 5 || r.CashDelayDays < 0 || r.CashDelayDays > 5 || r.QuoteTTLSeconds < 60 || r.QuoteTTLSeconds > 604800 {
		return fail("INVALID_RULES", "保证金、整手、交收或报价有效期无效")
	}
	if r.StampSide != "sell" && r.StampSide != "both" {
		return fail("INVALID_RULES", "印花税方向必须为 sell 或 both")
	}
	if r.Timezone == "" {
		return fail("INVALID_RULES", "无效 IANA 时区")
	}
	if _, err := MarketLocation(r.Timezone); err != nil {
		return fail("INVALID_RULES", "无效 IANA 时区")
	}
	if len(r.Sessions) < 1 || len(r.Sessions) > 4 || len(r.Holidays) > 400 {
		return fail("INVALID_RULES", "交易时段或休市日期数量无效")
	}
	lastEnd := -1
	for _, session := range r.Sessions {
		start, end, ok := parseMarketSession(session)
		if !ok || start < lastEnd {
			return fail("INVALID_RULES", "时段格式为 HH:MM-HH:MM，须按时间排序且不重叠")
		}
		lastEnd = end
	}
	for _, date := range r.Holidays {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return fail("INVALID_RULES", "休市日期格式为 YYYY-MM-DD")
		}
	}
	return nil
}

func ValidateMarketPresets(presets []MarketRules) error {
	if len(presets) == 0 {
		return fail("INVALID_RULES", "至少需要一个市场预设")
	}
	seen := make(map[string]bool, len(presets))
	for _, rules := range presets {
		if err := ValidateMarketRules(rules); err != nil {
			return err
		}
		if seen[rules.ID] {
			return fail("INVALID_RULES", "预设标识重复")
		}
		seen[rules.ID] = true
	}
	return nil
}

func LoadMarketRules(path string) (MarketRules, error) {
	var rules MarketRules
	file, err := os.Open(path)
	if err != nil {
		return rules, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&rules); err != nil {
		return rules, err
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return rules, fmt.Errorf("市场预设必须只包含一个 JSON 对象")
	}
	return rules, ValidateMarketRules(rules)
}

func LoadMarketPresets(directory string) ([]MarketRules, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	var presets []MarketRules
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		rules, err := LoadMarketRules(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		presets = append(presets, rules)
	}
	return presets, ValidateMarketPresets(presets)
}

func FeesFor(r MarketRules, side string, amount int64) Fees {
	fees := Fees{Commission: max(r.MinCommission, rate(amount, r.CommissionPPM)), Levy: rate(amount, r.LevyPPM)}
	if r.StampSide == "both" || r.StampSide == "sell" && (side == "sell" || side == "short") {
		fees.Stamp = rate(amount, r.StampPPM)
		fees.Stamp = (fees.Stamp + r.StampRound - 1) / r.StampRound * r.StampRound
	}
	return fees
}

func InitialMargin(notional int64, rules MarketRules) int64 {
	return rate(notional, rules.IMRPPM)
}
func MaintenanceMargin(notional int64, rules MarketRules) int64 {
	return rate(notional, rules.MMRPPM)
}

func parseMarketSession(value string) (int, int, bool) {
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	parse := func(value string) (int, bool) {
		if len(value) != 5 || value[2] != ':' {
			return 0, false
		}
		hour, hourError := strconv.Atoi(value[:2])
		minute, minuteError := strconv.Atoi(value[3:])
		if hourError != nil || minuteError != nil || hour < 0 || hour > 24 || minute < 0 || minute > 59 || hour == 24 && minute != 0 {
			return 0, false
		}
		return hour*60 + minute, true
	}
	start, validStart := parse(parts[0])
	end, validEnd := parse(parts[1])
	return start, end, validStart && validEnd && start < end
}

func marketDayAt(t time.Time, rules MarketRules) bool {
	if rules.WeekdaysOnly && (t.Weekday() == time.Saturday || t.Weekday() == time.Sunday) {
		return false
	}
	date := t.Format("2006-01-02")
	for _, holiday := range rules.Holidays {
		if holiday == date {
			return false
		}
	}
	return true
}

func MarketDay(now time.Time, rules MarketRules) bool {
	location, err := MarketLocation(rules.Timezone)
	return err == nil && marketDayAt(now.In(location), rules)
}

func IsMarketOpen(now time.Time, rules MarketRules) bool {
	location, err := MarketLocation(rules.Timezone)
	if err != nil {
		return false
	}
	local := now.In(location)
	if !marketDayAt(local, rules) {
		return false
	}
	minute := local.Hour()*60 + local.Minute()
	for _, session := range rules.Sessions {
		start, end, valid := parseMarketSession(session)
		if valid && minute >= start && minute < end {
			return true
		}
	}
	return false
}

// AvailableAt 返回 Unix 毫秒。T+N 以市场本地交易日计数，避开周末和显式节假日。
func AvailableAt(now time.Time, days int, rules MarketRules) int64 {
	if days <= 0 {
		return now.UnixMilli()
	}
	location, err := MarketLocation(rules.Timezone)
	if err != nil {
		return 0
	}
	local := now.In(location)
	local = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	for days > 0 {
		local = local.AddDate(0, 0, 1)
		if marketDayAt(local, rules) {
			days--
		}
	}
	return local.UnixMilli()
}

type SeasonWindow struct {
	ID       string `json:"id"`
	StartsAt int64  `json:"startsAt"`
	EndsAt   int64  `json:"endsAt"`
}

// SeasonFor 始终使用上海时间；赛季边界为 Unix 毫秒，与交易市场时区无关。
func SeasonFor(now time.Time, startDay, endDaysFromLast int) SeasonWindow {
	location, _ := MarketLocation("Asia/Shanghai")
	local := now.In(location)
	lastDay := time.Date(local.Year(), local.Month()+1, 0, 0, 0, 0, 0, location).Day()
	return SeasonWindow{ID: local.Format("2006-01"), StartsAt: time.Date(local.Year(), local.Month(), startDay, 0, 0, 0, 0, location).UnixMilli(), EndsAt: time.Date(local.Year(), local.Month(), lastDay-endDaysFromLast+1, 0, 0, 0, 0, location).UnixMilli()}
}
