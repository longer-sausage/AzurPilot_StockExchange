package exchange

import (
	"math/big"
	"time"
)

func effectiveRules(s Settings) MarketRules {
	r := s.ActiveMarket
	if r.ID == "meow" {
		r.CommissionPPM = s.CommissionPPM
		r.MinCommission = s.MinCommission
		r.BorrowAnnualPPM = s.BorrowAnnualPPM
		r.QuoteTTLSeconds = s.QuoteTTLSeconds
	}
	return r
}
func newTaxes(s Settings) bool { return s.ActiveMarket.ID == "meow" }
func gameSnapshot(s Settings) GameRules {
	return GameRules{TaxBrackets: append([]TaxBracket{}, s.TaxBrackets...), PremiumPPM: s.PremiumPPM}
}
func withGameSnapshot(s Settings, g GameRules) Settings {
	s.TaxBrackets = g.TaxBrackets
	s.PremiumPPM = g.PremiumPPM
	return s
}
func snapshotSettings(s Settings, r MarketRules) Settings {
	s.ActiveMarket = r
	if r.ID == "meow" {
		s.CommissionPPM = r.CommissionPPM
		s.MinCommission = r.MinCommission
		s.BorrowAnnualPPM = r.BorrowAnnualPPM
		s.QuoteTTLSeconds = r.QuoteTTLSeconds
	}
	return s
}
func nextMarketBoundary(now int64, s Settings) int64 {
	next := (now/3600000 + 1) * 3600000
	r := effectiveRules(s)
	location, _ := MarketLocation(r.Timezone)
	local := time.UnixMilli(now).In(location)
	for day := 0; day < 2; day++ {
		date := local.AddDate(0, 0, day)
		if !MarketDay(date, r) {
			continue
		}
		for _, session := range r.Sessions {
			start, end, _ := parseMarketSession(session)
			for _, minute := range []int{start, end} {
				candidate := time.Date(date.Year(), date.Month(), date.Day(), minute/60, minute%60, 0, 0, location).UnixMilli()
				if candidate > now && candidate < next {
					next = candidate
				}
			}
		}
	}
	return next
}
func availableCash(a *Account, now int64) int64 {
	v := a.Cash - a.Reserved
	for _, x := range a.Settlements {
		if x.AvailableAt > now {
			v -= x.Amount
		}
	}
	return max(int64(0), v)
}
func sellable(p *Position, now int64) int64 {
	if p == nil {
		return 0
	}
	if len(p.Lots) == 0 {
		return p.Quantity
	}
	var n int64
	for _, lot := range p.Lots {
		if lot.AvailableAt <= now {
			n += lot.Quantity
		}
	}
	return n
}
func leverageMargin(side string, leverage int64, r MarketRules) int64 {
	ppm := (1_000_000 + max(int64(1), leverage) - 1) / max(int64(1), leverage)
	if side == "short" {
		ppm = max(ppm, r.IMRPPM)
	}
	return ppm
}
func validLeverage(side string, leverage int64, r MarketRules) bool {
	if leverage == 0 {
		leverage = 1
	}
	return leverage >= 1 && leverage <= min(int64(10), max(int64(1), 1_000_000/r.IMRPPM)) && (!(side == "sell" || side == "cover") || leverage == 1)
}
func (m *mutation) marketOpen(now int64) (bool, string) {
	s := m.meta.Season
	if m.meta.Settings.Paused {
		return false, "管理员暂停交易"
	}
	if now < s.StartsAt {
		return false, "本月赛事尚未开始"
	}
	if now >= s.EndsAt || s.Settled {
		return false, "本月赛事已截止并结算"
	}
	if !IsMarketOpen(time.UnixMilli(now), effectiveRules(m.meta.Settings)) {
		return false, "当前为市场休市时段"
	}
	return true, "连续交易中"
}
func (m *mutation) settleCash(a *Account, now int64) {
	kept := a.Settlements[:0]
	for _, x := range a.Settlements {
		if x.AvailableAt > now {
			kept = append(kept, x)
		}
	}
	a.Settlements = kept
}
func carriedInterest(principal, annual, elapsed, remainder int64) (int64, int64) {
	const d = int64(365 * 86400 * 1000 * 1_000_000)
	n := new(big.Int).Mul(big.NewInt(principal), big.NewInt(annual))
	n.Mul(n, big.NewInt(elapsed))
	n.Add(n, big.NewInt(remainder))
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(n, big.NewInt(d), r)
	if !q.IsInt64() {
		return LedgerLimit, r.Int64()
	}
	return min(LedgerLimit, q.Int64()), r.Int64()
}
func (m *mutation) accrueFinancing(a *Account, stockID, now int64) error {
	p := a.Positions[stockID]
	if p == nil || p.Loan <= 0 {
		return nil
	}
	if p.FinancingAt == 0 {
		p.FinancingAt = now
		return nil
	}
	elapsed := now - p.FinancingAt
	if elapsed <= 0 {
		return nil
	}
	charge, remainder := carriedInterest(p.Loan, effectiveRules(m.meta.Settings).FinancingAnnualPPM, elapsed, p.FinancingRemainder)
	if err := m.recordFees(a, Revenue{Financing: charge}); err != nil {
		return err
	}
	paid := min(charge, availableCash(a, now))
	a.Cash -= paid
	p.Loan += charge - paid
	p.FinancingAt += elapsed
	p.FinancingRemainder = remainder
	return nil
}
func (m *mutation) closePosition(a *Account, s *Stock, now int64, reason string) error {
	for oid := range m.e.pending {
		old := m.e.orders[oid]
		if old.AccountID == a.ID && old.StockID == s.ID {
			o := m.order(oid)
			if o.Status == "pending" {
				m.cancel(a, o, now, reason)
			}
		}
	}
	p := a.Positions[s.ID]
	if p == nil {
		return nil
	}
	for _, side := range []string{"sell", "cover"} {
		quantity := p.Quantity
		if side == "cover" {
			quantity = p.ShortQuantity
		}
		if quantity <= 0 {
			continue
		}
		o := &Order{ID: m.meta.NextOrder, AccountID: a.ID, ClientID: "system-" + idstr(m.meta.NextOrder), StockID: s.ID, Side: side, Quantity: quantity, Remaining: quantity, Price: s.Price, Rules: effectiveRules(m.meta.Settings), Leverage: 1, Status: "pending", CreatedAt: now, UpdatedAt: now, Reason: reason}
		m.meta.NextOrder++
		m.orders[o.ID] = o
		m.changed = true
		if err := m.fill(a, s, o, now, true); err != nil {
			return err
		}
	}
	return nil
}
func (m *mutation) riskLong(a *Account, s *Stock, now int64) error {
	p := a.Positions[s.ID]
	if p == nil || p.Quantity == 0 || p.Loan <= 0 {
		return nil
	}
	if err := m.accrueFinancing(a, s.ID, now); err != nil {
		return err
	}
	value := p.Quantity * s.Price
	if value-p.Loan < rate(value, effectiveRules(m.meta.Settings).MMRPPM) {
		return m.closePosition(a, s, now, "融资持仓维持保证金不足，强制平仓")
	}
	return nil
}
func (m *mutation) advanceSeason(now int64) error {
	window := SeasonFor(time.UnixMilli(now), m.meta.Settings.StartDay, m.meta.Settings.EndDaysFromLast)
	current := m.meta.Season
	if current.ID == "" {
		m.meta.Season = Season{ID: window.ID, StartsAt: window.StartsAt, EndsAt: window.EndsAt}
		m.changed = true
		return nil
	}
	if !current.Settled && (now >= current.EndsAt || window.ID != current.ID) {
		cutoff := min(now, current.EndsAt)
		for id := range m.e.accounts {
			a := m.account(id)
			for sid := range a.Positions {
				s := m.stock(sid)
				if s != nil {
					if err := m.closePosition(a, s, cutoff, "月赛截止，按最后有效报价结算"); err != nil {
						return err
					}
				}
			}
			for oid := range m.e.pending {
				if o := m.e.orders[oid]; o.AccountID == id {
					fresh := m.order(oid)
					if fresh.Status == "pending" {
						m.cancel(a, fresh, cutoff, "月赛截止")
					}
				}
			}
			a.Settlements = nil
		}
		m.meta.Season.Settled = true
		archive := &SeasonArchive{Season: m.meta.Season, Rankings: []SeasonRank{}}
		all := map[int64]*Account{}
		for id := range m.e.accounts {
			all[id] = m.readAccount(id)
		}
		archive.Rankings = rankings(all, m.readStock, current.EndsAt)
		archive.Revenue = m.meta.Revenue
		m.archives[archive.Season.ID] = archive
		m.changed = true
	}
	if window.ID != current.ID {
		m.meta.Season = Season{ID: window.ID, StartsAt: window.StartsAt, EndsAt: window.EndsAt}
		for id := range m.e.accounts {
			a := m.account(id)
			a.MetricsSince = window.StartsAt
			a.LegacyMetrics = false
			a.Realized = 0
			a.Fees = Revenue{}
			a.Cash = m.meta.Settings.InitialCash
			a.InitialCash = a.Cash
			a.Reserved = 0
			a.Positions = map[int64]*Position{}
			a.Settlements = nil
		}
		for id := range m.e.stocks {
			s := m.stock(id)
			if s.DeletedAt != 0 {
				continue
			}
			s.Delisted = false
			s.Outstanding = 0
			s.Borrowed = 0
			s.NormalUsed = 0
			s.BorrowUsed = 0
		}
		m.meta.Revenue = Revenue{}
		m.changed = true
	}
	return nil
}
func (m *mutation) delistStock(s *Stock, now int64) error {
	season := m.meta.Season
	if s.Delisted || s.ObservedAt == 0 || s.Price >= m.meta.Settings.DelistThreshold*100 || now < season.StartsAt || now >= season.EndsAt || season.Settled {
		return nil
	}
	shanghai, _ := MarketLocation("Asia/Shanghai")
	if time.UnixMilli(now).In(shanghai).Day() < 5 {
		return nil
	}
	s.Delisted = true
	for id, old := range m.e.accounts {
		if p := old.Positions[s.ID]; p != nil && (p.Quantity > 0 || p.ShortQuantity > 0) {
			if err := m.closePosition(m.account(id), s, now, "行动力低于退市阈值，本月退市结算"); err != nil {
				return err
			}
		}
	}
	for oid := range m.e.pending {
		old := m.e.orders[oid]
		if old.StockID == s.ID {
			a := m.account(old.AccountID)
			o := m.order(oid)
			if o.Status == "pending" {
				m.cancel(a, o, now, "股票本月已退市")
			}
		}
	}
	m.changed = true
	return nil
}
