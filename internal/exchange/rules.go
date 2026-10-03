package exchange

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var locations sync.Map

// 时区规则不可变，避免每笔交易重新读取并解析系统时区文件。
func loadLocation(name string) (*time.Location, error) {
	if cached, ok := locations.Load(name); ok {
		return cached.(*time.Location), nil
	}
	l, err := time.LoadLocation(name)
	if err == nil {
		locations.Store(name, l)
	}
	return l, err
}

func rate(amount, ppm int64) int64 {
	return amount/1_000_000*ppm + (amount%1_000_000*ppm+999_999)/1_000_000
}
func FeesFor(r Rules, side string, amount int64) Fees {
	f := Fees{Commission: max(r.MinCommission, rate(amount, r.CommissionPPM)), Levy: rate(amount, r.LevyPPM)}
	if r.StampSide == "both" || r.StampSide == "sell" && (side == "sell" || side == "short") {
		f.Stamp = rate(amount, r.StampPPM)
		f.Stamp = (f.Stamp + r.StampRound - 1) / r.StampRound * r.StampRound
	}
	return f
}

var presetID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

func ValidateSettings(s Settings) error {
	if s.InitialCash < 100 || s.InitialCash > MaxNotional {
		return fail("INVALID_RULES", "初始资金须为 1–1,000,000,000,000 模拟币")
	}
	if s.StartDay < 1 || s.StartDay > 20 || s.EndDaysFromLast < 0 || s.EndDaysFromLast > 7 || len(s.Presets) == 0 || len(s.Presets) > 20 {
		return fail("INVALID_RULES", "月赛起止或预设数量无效")
	}
	ids := map[string]bool{}
	for _, r := range append(append([]Rules{}, s.Presets...), s.Active) {
		if !presetID.MatchString(r.ID) || len([]rune(r.Name)) < 1 || len([]rune(r.Name)) > 40 || len(r.Description) > 1500 || len(r.Source) > 500 {
			return fail("INVALID_RULES", "预设名称或标识无效")
		}
		if r.Source != "" {
			u, err := url.Parse(r.Source)
			if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
				return fail("INVALID_RULES", "参考来源须为 HTTPS URL")
			}
		}
		for _, n := range []int64{r.CommissionPPM, r.StampPPM, r.LevyPPM, r.BorrowAnnualPPM, r.FinancingAnnualPPM} {
			if n < 0 || n > 1_000_000 {
				return fail("INVALID_RULES", "费率须在 0% 到 100% 之间")
			}
		}
		if r.MinCommission < 0 || r.MinCommission > 1_000_000 || r.IMRPPM < 100_000 || r.IMRPPM > 2_000_000 || r.MMRPPM < 50_000 || r.MMRPPM > r.IMRPPM || r.LotSize < 1 || r.LotSize > 10000 || r.StampRound < 1 || r.StampRound > 10000 || r.SellDelayDays < 0 || r.SellDelayDays > 5 || r.CashDelayDays < 0 || r.CashDelayDays > 5 || r.QuoteTTLSeconds < 60 || r.QuoteTTLSeconds > 604800 {
			return fail("INVALID_RULES", "保证金、整手、交收或报价有效期无效")
		}
		if r.StampSide != "sell" && r.StampSide != "both" {
			return fail("INVALID_RULES", "印花税方向必须为 sell 或 both")
		}
		if _, err := loadLocation(r.Timezone); err != nil {
			return fail("INVALID_RULES", "无效 IANA 时区")
		}
		if len(r.Sessions) < 1 || len(r.Sessions) > 4 || len(r.Holidays) > 400 {
			return fail("INVALID_RULES", "交易时段或休市日期数量无效")
		}
		lastEnd := -1
		for _, session := range r.Sessions {
			a, b, ok := parseSession(session)
			if !ok || a < lastEnd {
				return fail("INVALID_RULES", "时段格式为 HH:MM-HH:MM，须按时间排序且不重叠")
			}
			lastEnd = b
		}
		for _, date := range r.Holidays {
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return fail("INVALID_RULES", "休市日期格式为 YYYY-MM-DD")
			}
		}
	}
	for _, r := range s.Presets {
		if ids[r.ID] {
			return fail("INVALID_RULES", "预设标识重复")
		}
		ids[r.ID] = true
	}
	return nil
}
func parseSession(s string) (int, int, bool) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	parse := func(v string) (int, bool) {
		if len(v) != 5 || v[2] != ':' {
			return 0, false
		}
		h, e := strconv.Atoi(v[:2])
		m, e2 := strconv.Atoi(v[3:])
		if e != nil || e2 != nil || h < 0 || h > 24 || m < 0 || m > 59 || h == 24 && m != 0 {
			return 0, false
		}
		return h*60 + m, true
	}
	a, ok := parse(parts[0])
	b, ok2 := parse(parts[1])
	return a, b, ok && ok2 && a < b
}
func marketDay(t time.Time, r Rules) bool {
	if r.WeekdaysOnly && (t.Weekday() == time.Saturday || t.Weekday() == time.Sunday) {
		return false
	}
	for _, h := range r.Holidays {
		if h == t.Format("2006-01-02") {
			return false
		}
	}
	return true
}
func sessionOpen(now time.Time, r Rules) bool {
	l, _ := loadLocation(r.Timezone)
	t := now.In(l)
	if !marketDay(t, r) {
		return false
	}
	m := t.Hour()*60 + t.Minute()
	for _, s := range r.Sessions {
		a, b, _ := parseSession(s)
		if m >= a && m < b {
			return true
		}
	}
	return false
}
func availableAt(now time.Time, days int, r Rules) int64 {
	if days == 0 {
		return now.Unix()
	}
	l, _ := loadLocation(r.Timezone)
	t := now.In(l)
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, l)
	for days > 0 {
		t = t.AddDate(0, 0, 1)
		if marketDay(t, r) {
			days--
		}
	}
	return t.Unix()
}
func seasonFor(now time.Time, s Settings) Season {
	// 月赛统一使用上海时间；市场时区仅影响交易时段和 T+N。
	l, _ := loadLocation("Asia/Shanghai")
	t := now.In(l)
	last := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, l).Day()
	return Season{ID: t.Format("2006-01"), StartsAt: time.Date(t.Year(), t.Month(), s.StartDay, 0, 0, 0, 0, l).Unix(), EndsAt: time.Date(t.Year(), t.Month(), last-s.EndDaysFromLast+1, 0, 0, 0, 0, l).Unix()}
}
func (e *Engine) isOpen(now time.Time) (bool, string) {
	if e.state.Settings.Paused {
		return false, "管理员暂停交易"
	}
	if now.Unix() < e.state.Season.StartsAt {
		return false, "本月赛事尚未开始"
	}
	if now.Unix() >= e.state.Season.EndsAt || e.state.Season.Settled {
		return false, "本月赛事已截止并结算"
	}
	if !sessionOpen(now, e.state.Settings.Active) {
		return false, "当前为市场休市时段"
	}
	return true, "连续交易中"
}
func symbol(id int64) string { return fmt.Sprintf("MM%06d", id) }
