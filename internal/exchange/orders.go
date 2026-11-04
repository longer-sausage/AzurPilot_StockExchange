package exchange

import (
	"math/big"
	"sort"
	"strings"
	"time"
)

func taxRate(s Settings, assets int64) int64 {
	for _, b := range s.TaxBrackets {
		if b.UpTo == 0 || assets <= b.UpTo {
			return b.RatePPM
		}
	}
	return 0
}
func maximumTax(s Settings) int64 {
	var n int64
	for _, b := range s.TaxBrackets {
		n = max(n, b.RatePPM)
	}
	return n
}
func commission(s Settings, amount int64) int64 {
	return max(s.MinCommission, rate(amount, s.CommissionPPM))
}
func buying(side string) bool    { return side == "buy" || side == "cover" }
func borrowing(side string) bool { return side == "short" || side == "cover" }
func immediateOrder(o *Order) bool {
	return o.Kind == "market" || (o.Kind == "stop" && o.Triggered)
}

// 分量分别按比例向下取整；最后一股取尽余数。冻结、可付金额和落账必须复用同一公式。
func portion(value, quantity, total int64) int64 {
	if quantity == total {
		return value
	}
	return value/total*quantity + (value%total)*quantity/total
}
func shortEscrow(p *Position, quantity int64) (int64, int64) {
	return portion(p.ShortProceeds, quantity, p.ShortQuantity), portion(p.Collateral, quantity, p.ShortQuantity)
}
func executionPrice(s *Stock, side string, quantity int64, rules Settings) (int64, int64) {
	if !newTaxes(rules) {
		return s.Price, 0
	}
	used, limit := s.NormalUsed, s.HourlyVolume
	if borrowing(side) {
		used, limit = s.BorrowUsed, s.BorrowHourlyVolume
	}
	utilization := min(int64(1_000_000), (used*1_000_000+quantity*500000)/max(int64(1), limit))
	premium := rules.PremiumPPM * utilization / 1_000_000
	adjust := rate(s.Price, premium)
	if buying(side) {
		return min(MaxPrice, s.Price+adjust), premium
	}
	return max(int64(1), s.Price-adjust), premium
}
func reserveFor(in OrderInput, rules Settings, p *Position) int64 {
	r := effectiveRules(rules)
	notional := in.Price * in.Quantity
	fees := FeesFor(r, in.Side, notional)
	fees.Commission = max(fees.Commission, r.MinCommission*((in.Quantity+r.LotSize-1)/r.LotSize))
	tax := int64(0)
	if newTaxes(rules) {
		tax = rate(notional, maximumTax(rules))
	}
	if in.Side == "sell" {
		return 0
	}
	reserve := notional + tax + fees.Total()
	if in.Side == "buy" || in.Side == "short" {
		reserve = rate(notional, leverageMargin(in.Side, in.Leverage, r)) + tax + fees.Total()
	}
	if in.Side == "cover" && p != nil && p.ShortQuantity > 0 {
		proceeds, collateral := shortEscrow(p, in.Quantity)
		return max(int64(0), reserve-proceeds-collateral)
	}
	return reserve
}
func (m *mutation) pendingQuantity(accountID, stockID int64, side string, except int64) int64 {
	var n int64
	for id := range m.e.pending {
		old := m.e.orders[id]
		o := old
		if fresh := m.orders[id]; fresh != nil {
			o = fresh
		}
		if id != except && o.AccountID == accountID && o.StockID == stockID && o.Side == side && o.Status == "pending" {
			n += o.Remaining
		}
	}
	return n
}
func (m *mutation) validateOrder(a *Account, s *Stock, in OrderInput) error {
	if a == nil || a.Disabled {
		return fail("UNAUTHORIZED", "账户不可用")
	}
	if err := accountReady(a, m.readStock); err != nil {
		return err
	}
	if m.meta.Settings.Paused {
		return fail("MARKET_PAUSED", "交易所暂停交易")
	}
	if open, reason := m.marketOpen(m.e.clock().UnixMilli()); !open {
		return fail("MARKET_CLOSED", reason)
	}
	r := effectiveRules(m.meta.Settings)
	if !validLeverage(in.Side, in.Leverage, r) {
		return fail("INVALID_LEVERAGE", "杠杆超过当前初始保证金规则允许范围")
	}
	if a.BoundStockID == s.ID {
		return fail("SELF_TRADE", "不能交易自己绑定的股票")
	}
	if s.DeletedAt != 0 {
		return fail("STOCK_DELETED", "股票已被管理员删除")
	}
	if s.Disabled {
		return fail("STOCK_DISABLED", "股票已停牌")
	}
	if s.Delisted {
		return fail("STOCK_DELISTED", "该股票已退市")
	}
	if s.Price <= 0 {
		return fail("ZERO_QUOTE", "股票尚无有效报价")
	}
	if m.e.clock().UnixMilli()-s.ObservedAt > r.QuoteTTLSeconds*1000 {
		return fail("STALE_QUOTE", "报价已过期，等待实例同步")
	}
	if in.Kind != "limit" && in.Kind != "market" && in.Kind != "stop" {
		return fail("INVALID_ORDER", "委托类型无效")
	}
	if in.TimeInForce != "DAY" && in.TimeInForce != "GTC" {
		return fail("INVALID_ORDER", "委托有效期无效")
	}
	if in.Side != "buy" && in.Side != "sell" && in.Side != "short" && in.Side != "cover" {
		return fail("INVALID_ORDER", "交易方向无效")
	}
	if in.Quantity < 1 || in.Quantity > LedgerLimit/MaxPrice/8 || in.Price < 1 || in.Price > MaxPrice || len(in.ClientID) < 8 || len(in.ClientID) > 128 || strings.ContainsAny(in.ClientID, "\r\n\x00") {
		return fail("INVALID_ORDER", "数量、整数分价格或幂等标识无效")
	}
	if in.Side == "short" && s.AccountID == 0 {
		return fail("SHORT_UNBOUND", "仅允许卖空已绑定交易账户的股票")
	}
	if in.Quantity%r.LotSize != 0 {
		return fail("INVALID_LOT", "数量须符合当前市场整手单位")
	}
	p := a.Positions[s.ID]
	if p != nil && ((in.Side == "buy" && p.ShortQuantity > 0) || (in.Side == "short" && p.Quantity > 0)) {
		return fail("OPPOSITE_POSITION", "请先平掉相反方向的持仓")
	}
	if (in.Side == "buy" && m.pendingQuantity(a.ID, s.ID, "short", 0) > 0) || (in.Side == "short" && m.pendingQuantity(a.ID, s.ID, "buy", 0) > 0) {
		return fail("OPPOSITE_POSITION", "请先撤销相反方向开仓挂单")
	}
	if in.Side == "sell" {
		if p == nil || sellable(p, m.e.clock().UnixMilli())-m.pendingQuantity(a.ID, s.ID, "sell", 0) < in.Quantity {
			return fail("INSUFFICIENT_POSITION", "可卖持仓不足")
		}
	}
	if in.Side == "cover" {
		if p == nil || p.ShortQuantity-m.pendingQuantity(a.ID, s.ID, "cover", 0) < in.Quantity {
			return fail("INSUFFICIENT_SHORT", "可回补数量不足")
		}
	}
	// 预留全部现有持仓与待开仓在最高报价下的累计整数空间。
	budget := LedgerLimit / MaxPrice / 8
	for _, pos := range a.Positions {
		if pos.Quantity > budget || pos.ShortQuantity > budget-pos.Quantity {
			return fail("NUMERICAL_LIMIT", "账户累计持仓超出整数安全范围")
		}
		budget -= pos.Quantity + pos.ShortQuantity
	}
	for oid := range m.e.pending {
		o := m.e.orders[oid]
		if fresh := m.orders[oid]; fresh != nil {
			o = fresh
		}
		if o.AccountID == a.ID && o.Status == "pending" && (o.Side == "buy" || o.Side == "short") {
			if o.Remaining > budget {
				return fail("NUMERICAL_LIMIT", "账户累计挂单超出整数安全范围")
			}
			budget -= o.Remaining
		}
	}
	if (in.Side == "buy" || in.Side == "short") && in.Quantity > budget {
		return fail("NUMERICAL_LIMIT", "账户累计数量超出整数安全范围")
	}
	reserve := reserveFor(in, m.meta.Settings, p)
	if reserve > availableCash(a, m.e.clock().UnixMilli()) {
		return fail("INSUFFICIENT_CASH", "可用现金不足（含最高税档和部分成交费用预留）")
	}
	return nil
}
func (e *Engine) Preview(id int64, in OrderInput) (*Preview, error) {
	in.Leverage = max(int64(1), in.Leverage)
	if in.Kind == "" {
		in.Kind = "limit"
	}
	if in.TimeInForce == "" {
		in.TimeInForce = "GTC"
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	s, err := e.resolve(in.Stock, id)
	if err != nil {
		return nil, err
	}
	m := &mutation{e: e, meta: e.meta, orders: map[int64]*Order{}}
	a := e.accounts[id]
	if in.Kind == "market" {
		in.Price = s.Price
	}
	if o := e.orders[e.byClient[orderKey(id, in.ClientID)]]; o != nil {
		if o.StockID != s.ID || o.Side != in.Side || o.Quantity != in.Quantity || (in.Kind != "market" && o.Price != in.Price) || o.Leverage != in.Leverage || o.Kind != in.Kind || o.TimeInForce != in.TimeInForce {
			return nil, fail("IDEMPOTENCY_CONFLICT", "同一幂等标识不能用于不同委托")
		}
	}
	if err = m.validateOrder(a, s, in); err != nil {
		return nil, err
	}
	assets := equity(a, func(id int64) *Stock { return e.stocks[id] })
	notional := in.Quantity * in.Price
	tr := int64(0)
	if newTaxes(e.meta.Settings) {
		tr = taxRate(e.meta.Settings, assets)
	}
	tax := rate(notional, tr)
	marketRules := effectiveRules(e.meta.Settings)
	fees := FeesFor(marketRules, in.Side, notional)
	fee := fees.Commission
	total := notional + tax + fees.Total()
	if in.Side == "sell" {
		total = notional - tax - fees.Total()
	}
	if in.Side == "buy" || in.Side == "short" {
		total = rate(notional, leverageMargin(in.Side, in.Leverage, marketRules)) + tax + fees.Total()
	}
	_, premium := executionPrice(s, in.Side, min(in.Quantity, max(int64(1), s.HourlyVolume)), e.meta.Settings)
	p := &Preview{Kind: in.Kind, TimeInForce: in.TimeInForce, Leverage: max(int64(1), in.Leverage), Rules: marketRules, Stamp: fees.Stamp, Levy: fees.Levy, PreviewID: randomToken(24), ExpiresAt: e.clock().UnixMilli() + 300000, Stock: publicStock(s), Side: in.Side, Quantity: in.Quantity, Price: in.Price, Notional: notional, TaxRatePPM: tr, Tax: tax, Commission: fee, EstimatedTotal: total, PremiumPPM: premium, AccountID: id, Input: in}
	if in.Side == "sell" {
		pos := a.Positions[s.ID]
		p.Repayment = portion(pos.Loan, in.Quantity, pos.Quantity)
		p.EstimatedTotal -= p.Repayment
	}
	p.GameRules = gameSnapshot(e.meta.Settings)
	if in.Side == "buy" {
		p.Financing = notional - rate(notional, leverageMargin(in.Side, in.Leverage, marketRules))
	}
	if in.Side == "short" {
		p.Collateral = rate(notional, leverageMargin(in.Side, in.Leverage, marketRules))
	}
	e.previews[p.PreviewID] = p
	copy := *p
	return &copy, nil
}
func (e *Engine) Confirm(id int64, previewID string) (*Order, error) {
	var out *Order
	err := e.transaction(func(m *mutation) error {
		if err := accountReady(m.readAccount(id), m.readStock); err != nil {
			return err
		}
		if o := e.orders[e.byPreview[orderKey(id, previewID)]]; o != nil {
			x := *o
			out = &x
			return nil
		}
		p := e.previews[previewID]
		if p == nil || p.AccountID != id || p.ExpiresAt < e.clock().UnixMilli() {
			return fail("PREVIEW_EXPIRED", "交易确认已过期，请重新预览")
		}
		if p.Rules.ID != m.meta.Settings.ActiveMarket.ID {
			return fail("PREVIEW_RULES_CHANGED", "市场预设已切换，请重新预览交易")
		}
		s, err := e.resolve(p.Input.Stock, id)
		if err != nil {
			return err
		}
		if o := e.orders[e.byClient[orderKey(id, p.Input.ClientID)]]; o != nil {
			if o.StockID != s.ID || o.Side != p.Side || o.Quantity != p.Quantity || (p.Kind != "market" && o.Price != p.Price) || o.Leverage != p.Leverage || o.Kind != p.Kind || o.TimeInForce != p.TimeInForce {
				return fail("IDEMPOTENCY_CONFLICT", "幂等标识对应不同委托")
			}
			x := *o
			out = &x
			return nil
		}
		a := m.readAccount(id)
		if err := m.auction(e.clock().UnixMilli()); err != nil {
			return err
		}
		a = m.readAccount(id)
		currentSettings := m.meta.Settings
		err = m.validateOrder(a, s, p.Input)
		if publicErrorOptionalCode(err) == "INSUFFICIENT_CASH" {
			snapshot := withGameSnapshot(snapshotSettings(currentSettings, p.Rules), p.GameRules)
			reserve := reserveFor(p.Input, snapshot, a.Positions[s.ID])
			if reserve <= availableCash(a, e.clock().UnixMilli()) {
				err = nil
			}
		}
		if err != nil {
			return err
		}
		reserved := reserveFor(p.Input, withGameSnapshot(snapshotSettings(m.meta.Settings, p.Rules), p.GameRules), a.Positions[s.ID])
		if reserved > availableCash(a, e.clock().UnixMilli()) {
			return fail("INSUFFICIENT_CASH", "可用现金不足以冻结已确认费用快照")
		}
		a = m.account(id)
		expires := int64(0)
		if p.TimeInForce == "DAY" && p.Kind != "market" {
			loc, _ := MarketLocation(p.Rules.Timezone)
			now := e.clock().In(loc)
			expires = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc).UnixMilli()
		}
		o := &Order{Kind: p.Kind, TimeInForce: p.TimeInForce, ExpiresAt: expires, Leverage: p.Leverage, Rules: p.Rules, ID: m.meta.NextOrder, AccountID: id, ClientID: p.Input.ClientID, PreviewID: previewID, StockID: s.ID, Side: p.Side, Quantity: p.Quantity, Remaining: p.Quantity, Price: p.Price, Status: "pending", CreatedAt: e.clock().UnixMilli(), UpdatedAt: e.clock().UnixMilli(), EligibleHour: e.clock().Unix()/3600 + 1}
		m.meta.NextOrder++
		o.GameRules = p.GameRules
		o.Reserved = reserved
		a.Reserved += o.Reserved
		m.orders[o.ID] = o
		m.changed = true
		s = m.stock(s.ID)
		m.refreshHour(s, e.clock().UnixMilli())
		if err = m.fill(a, s, o, e.clock().UnixMilli(), false); err != nil {
			return err
		}
		x := *o
		out = &x
		return nil
	})
	return out, err
}
func (m *mutation) refreshHour(s *Stock, now int64) {
	hour := now / 3600000
	if s.Hour != hour {
		s.Hour = hour
		s.NormalUsed = 0
		s.BorrowUsed = 0
	}
}
func (m *mutation) release(a *Account, o *Order) { a.Reserved -= o.Reserved; o.Reserved = 0 }
func (m *mutation) cancel(a *Account, o *Order, now int64, reason string) {
	m.release(a, o)
	o.Status = "cancelled"
	o.Reason = reason
	o.UpdatedAt = now
}
func (e *Engine) Cancel(id, orderID int64) (*Order, error) {
	var out *Order
	err := e.transaction(func(m *mutation) error {
		o := e.orders[orderID]
		if o == nil || o.AccountID != id {
			return fail("NOT_FOUND", "委托不存在")
		}
		if o.Status != "pending" {
			x := *o
			out = &x
			return nil
		}
		a := m.account(id)
		o = m.order(orderID)
		m.cancel(a, o, e.clock().UnixMilli(), "用户取消")
		x := *o
		out = &x
		return nil
	})
	return out, err
}
func (m *mutation) fill(a *Account, s *Stock, o *Order, now int64, forced bool) error {
	rules := m.meta.Settings
	currentRules := effectiveRules(rules)
	if len(o.GameRules.TaxBrackets) > 0 {
		rules = withGameSnapshot(rules, o.GameRules)
	}
	r := o.Rules
	if r.ID == "" {
		r = effectiveRules(rules)
	}
	if o.Remaining <= 0 {
		return nil
	}
	if !forced {
		// 市价委托仅尝试本次撮合，包括零成交；解冻与成交在同一事务提交。
		defer func() {
			if o.Status == "pending" && immediateOrder(o) {
				m.cancel(a, o, now, "市价委托未成交余量自动撤销")
			}
		}()
	}
	if !forced && (a.BoundStockID == 0 || m.readStock(a.BoundStockID) == nil || m.readStock(a.BoundStockID).DeletedAt != 0) {
		m.cancel(a, o, now, "账户未绑定股票，撤销委托")
		return nil
	}
	if !forced && a.BoundStockID == s.ID {
		m.cancel(a, o, now, "账户已绑定该股票，禁止自我交易")
		return nil
	}
	if !forced {
		open, _ := m.marketOpen(now)
		if !open || a.Disabled || a.ForcePasswordChange || s.Disabled || s.DeletedAt != 0 || s.Delisted || s.Price <= 0 || now-s.ObservedAt > effectiveRules(rules).QuoteTTLSeconds*1000 {
			return nil
		}
		if !validLeverage(o.Side, o.Leverage, effectiveRules(rules)) {
			m.cancel(a, o, now, "杠杆已超过当前保证金允许范围")
			return nil
		}
	}
	if err := m.accrue(a, s.ID, now); err != nil {
		return err
	}
	m.settleCash(a, now)
	if !forced && o.Kind == "stop" && !o.Triggered {
		if (buying(o.Side) && s.Price >= o.Price) || (!buying(o.Side) && s.Price <= o.Price) {
			o.Triggered = true
		} else {
			return nil
		}
	}
	quantity := o.Remaining
	available := s.HourlyVolume - s.NormalUsed
	if borrowing(o.Side) {
		available = s.BorrowHourlyVolume - s.BorrowUsed
	}
	if !forced {
		quantity = min(quantity, max(int64(0), available))
	}
	if o.Side == "buy" {
		quantity = min(quantity, max(int64(0), s.IssuedShares-s.Outstanding))
	}
	if o.Side == "short" {
		if s.AccountID == 0 {
			return nil
		}
		quantity = min(quantity, max(int64(0), s.BorrowShares-s.Borrowed))
	}
	p := a.Positions[s.ID]
	if !forced && p != nil && ((o.Side == "buy" && p.ShortQuantity > 0) || (o.Side == "short" && p.Quantity > 0)) {
		m.cancel(a, o, now, "相反方向持仓冲突")
		o.Status = "rejected"
		return nil
	}
	if p == nil {
		p = &Position{StockID: s.ID, BorrowAt: now}
		a.Positions[s.ID] = p
	}
	if o.Side == "sell" {
		if forced {
			quantity = min(quantity, p.Quantity)
		} else {
			quantity = min(quantity, sellable(p, now))
		}
	}
	if o.Side == "cover" {
		quantity = min(quantity, p.ShortQuantity)
	}
	lot := int64(1)
	if !forced {
		lot = r.LotSize
		quantity = quantity / lot * lot
	}
	if quantity <= 0 {
		return nil
	}
	price, _ := executionPrice(s, o.Side, quantity, rules)
	if forced {
		price = s.Price
	}
	if !forced && o.Kind != "market" && o.Kind != "stop" && ((buying(o.Side) && price > o.Price) || (!buying(o.Side) && price < o.Price)) {
		// 整批溢价越过限价时，仍成交符合限价的最大整手数量。
		// 数量越小，买入/回补报价越低，卖出/卖空报价越高。
		lo, hi := int64(0), quantity/lot
		for lo < hi {
			mid := (lo + hi + 1) / 2
			candidate, _ := executionPrice(s, o.Side, mid*lot, rules)
			if (buying(o.Side) && candidate <= o.Price) || (!buying(o.Side) && candidate >= o.Price) {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		quantity = lo * lot
		if quantity == 0 {
			return nil
		}
		price, _ = executionPrice(s, o.Side, quantity, rules)
	}
	tr := int64(0)
	if newTaxes(rules) {
		tr = taxRate(rules, equity(a, m.readStock))
	}
	calc := func(q int64) (int64, int64, Fees) {
		amount := q * price
		return amount, rate(amount, tr), FeesFor(r, o.Side, amount)
	}
	free := availableCash(a, now) + o.Reserved
	ownPPM := leverageMargin(o.Side, o.Leverage, currentRules)
	cost := func(q int64) int64 {
		amount, t, f := calc(q)
		switch o.Side {
		case "buy", "short":
			return rate(amount, ownPPM) + t + f.Total()
		case "cover":
			proceeds, collateral := shortEscrow(p, q)
			return max(int64(0), amount+t+f.Total()-proceeds-collateral)
		case "sell":
			repayment := portion(p.Loan, q, p.Quantity)
			return max(int64(0), repayment+t+f.Total()-amount)
		}
		return 0
	}
	if !forced && cost(quantity) > free {
		lo, hi := int64(0), quantity/lot
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if cost(mid*lot) <= free {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		quantity = lo * lot
		if quantity == 0 {
			m.cancel(a, o, now, "现金不足以支付成交费用或融资本金")
			o.Status = "rejected"
			return nil
		}
	}
	notional, tax, fees := calc(quantity)
	m.release(a, o)
	feeBudget := tax + fees.Total()
	switch o.Side {
	case "buy":
		own := rate(notional, ownPPM)
		a.Cash -= own + tax + fees.Total()
		p.Loan += notional - own
		p.FinancingAt = now
		p.Quantity += quantity
		p.Cost += notional + tax + fees.Total()
		p.Lots = append(p.Lots, Lot{quantity, AvailableAt(time.UnixMilli(now), r.SellDelayDays, r)})
		s.Outstanding += quantity
	case "sell":
		repayment := portion(p.Loan, quantity, p.Quantity)
		net := notional - repayment - tax - fees.Total()
		if forced {
			feeBudget = min(feeBudget, max(int64(0), notional+a.Cash-a.Reserved-repayment))
		}
		if forced && net < -(a.Cash-a.Reserved) {
			net = -(a.Cash - a.Reserved)
		}
		a.Cash += net
		p.Loan -= repayment
		closedCost := portion(p.Cost, quantity, p.Quantity)
		a.Realized += notional - tax - fees.Total() - closedCost
		p.Cost -= closedCost
		p.Quantity -= quantity
		s.Outstanding -= quantity
		left := quantity
		for i := range p.Lots {
			entry := &p.Lots[i]
			if entry.AvailableAt <= now || forced {
				take := min(left, entry.Quantity)
				entry.Quantity -= take
				left -= take
			}
		}
		kept := p.Lots[:0]
		for _, entry := range p.Lots {
			if entry.Quantity > 0 {
				kept = append(kept, entry)
			}
		}
		p.Lots = kept
		if p.Loan == 0 {
			p.FinancingRemainder = 0
		}
		if r.CashDelayDays > 0 && !forced && net > 0 {
			a.Settlements = append(a.Settlements, Settlement{net, AvailableAt(time.UnixMilli(now), r.CashDelayDays, r)})
		}
	case "short":
		collateral := rate(notional, ownPPM)
		a.Cash -= collateral + tax + fees.Total()
		p.ShortQuantity += quantity
		p.ShortProceeds += notional
		p.ShortCost += notional - tax - fees.Total()
		p.Collateral += collateral
		p.BorrowAt = now
		s.Borrowed += quantity
	case "cover":
		proceeds, collateral := shortEscrow(p, quantity)
		totalCost := notional + tax + fees.Total()
		funds := proceeds + collateral + max(int64(0), a.Cash-a.Reserved)
		if forced && totalCost > funds {
			totalCost = funds
		}
		if forced {
			feeBudget = min(feeBudget, max(int64(0), totalCost-notional))
		}
		a.Cash += proceeds + collateral - totalCost
		loss := max(int64(0), notional-proceeds)
		if forced {
			loss = min(loss, max(int64(0), totalCost-proceeds))
		}
		if loss > 0 && s.AccountID != 0 {
			issuer := m.account(s.AccountID)
			if issuer == nil {
				return fail("INVALID_ISSUER", "股票绑定账户缺失")
			}
			if issuer.Cash > LedgerLimit-loss {
				return fail("NUMERICAL_LIMIT", "发行者现金超出安全范围")
			}
			issuer.Cash += loss
		}
		closedCost := portion(p.ShortCost, quantity, p.ShortQuantity)
		a.Realized += closedCost - notional - tax - fees.Total()
		p.ShortCost -= closedCost
		p.ShortQuantity -= quantity
		p.ShortProceeds -= proceeds
		p.Collateral -= collateral
		if p.ShortQuantity == 0 {
			p.BorrowRemainder = 0
		}
		s.Borrowed -= quantity
	}
	if forced && feeBudget < tax+fees.Total() {
		oldFees := tax + fees.Total()
		take := func(value int64) int64 { paid := min(value, feeBudget); feeBudget -= paid; return paid }
		tax = take(tax)
		fees.Commission = take(fees.Commission)
		fees.Stamp = take(fees.Stamp)
		fees.Levy = take(fees.Levy)
		a.Realized += oldFees - tax - fees.Total()
	}
	feeRecord := Revenue{Tax: tax, Commission: fees.Commission, Stamp: fees.Stamp, Levy: fees.Levy}
	if err := m.recordFees(a, feeRecord); err != nil {
		return err
	}
	t := &Trade{ID: m.meta.NextTrade, OrderID: o.ID, StockID: s.ID, AccountID: a.ID, Username: a.Username, Side: o.Side, Kind: o.Kind, Quantity: quantity, Price: price, Notional: notional, At: now, Forced: forced, Reason: o.Reason, Fees: feeRecord}
	m.meta.NextTrade++
	m.trades = append(m.trades, t)
	o.TradeRecorded = true
	s.TradeVersion++
	o.Notional += notional
	o.Filled += quantity
	o.Remaining -= quantity
	o.AveragePrice = o.Notional / o.Filled
	o.Tax += tax
	o.Commission += fees.Commission
	o.Stamp += fees.Stamp
	o.Levy += fees.Levy
	o.TaxRatePPM = tr
	o.UpdatedAt = now
	if borrowing(o.Side) {
		s.BorrowUsed += quantity
	} else {
		s.NormalUsed += quantity
	}
	if o.Remaining == 0 {
		o.Status = "filled"
	} else if forced || !immediateOrder(o) {
		o.Status = "pending"
		in := OrderInput{Kind: o.Kind, TimeInForce: o.TimeInForce, Side: o.Side, Quantity: o.Remaining, Price: o.Price, Leverage: o.Leverage}
		snapshot := rules
		snapshot.ActiveMarket = r
		if r.ID == "meow" {
			snapshot.CommissionPPM = r.CommissionPPM
			snapshot.MinCommission = r.MinCommission
		}
		o.Reserved = min(availableCash(a, now), reserveFor(in, snapshot, p))
		a.Reserved += o.Reserved
	}
	return nil
}
func (m *mutation) accrue(a *Account, stockID, now int64) error {
	if err := m.accrueFinancing(a, stockID, now); err != nil {
		return err
	}
	p := a.Positions[stockID]
	if p == nil || p.ShortQuantity == 0 {
		return nil
	}
	if p.BorrowAt == 0 {
		p.BorrowAt = now
		return nil
	}
	elapsed := now - p.BorrowAt
	if elapsed <= 0 {
		return nil
	}
	s := m.readStock(stockID)
	if s == nil {
		return nil
	}
	const denominator = int64(365 * 86400 * 1000 * 1_000_000)
	numerator := new(big.Int).Mul(big.NewInt(p.ShortQuantity*s.Price), big.NewInt(effectiveRules(m.meta.Settings).BorrowAnnualPPM))
	numerator.Mul(numerator, big.NewInt(elapsed))
	numerator.Add(numerator, big.NewInt(p.BorrowRemainder))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, big.NewInt(denominator), remainder)
	charge := int64(0)
	if !quotient.IsInt64() {
		charge = LedgerLimit
	} else {
		charge = quotient.Int64()
	}
	charge = min(charge, availableCash(a, now)+p.Collateral)
	cashCharge := min(charge, availableCash(a, now))
	if err := m.recordFees(a, Revenue{Borrow: charge}); err != nil {
		return err
	}
	a.Cash -= cashCharge
	p.Collateral -= charge - cashCharge
	p.BorrowAt += elapsed
	p.BorrowRemainder = remainder.Int64()
	return nil
}
func (m *mutation) liquidateStock(s *Stock, now int64) error {
	if s.Price <= 0 {
		return nil
	}
	for id := range m.e.shortOwners[s.ID] {
		old := m.readAccount(id)
		position := old.Positions[s.ID]
		if position == nil || position.ShortQuantity == 0 {
			continue
		}
		a := m.account(id)
		if err := m.accrue(a, s.ID, now); err != nil {
			return err
		}
		p := a.Positions[s.ID]
		liability := p.ShortQuantity * s.Price
		if p.ShortProceeds+p.Collateral-liability >= rate(liability, effectiveRules(m.meta.Settings).MMRPPM) {
			continue
		}
		for oid := range m.e.pending {
			oldOrder := m.e.orders[oid]
			if oldOrder.AccountID == id && oldOrder.Status == "pending" {
				o := m.order(oid)
				m.cancel(a, o, now, "保证金不足，强制平仓前取消挂单")
			}
		}
		o := &Order{ID: m.meta.NextOrder, AccountID: id, ClientID: "liquidation-" + idstr(m.meta.NextOrder), StockID: s.ID, Side: "cover", Quantity: p.ShortQuantity, Remaining: p.ShortQuantity, Price: s.Price, Status: "pending", CreatedAt: now, UpdatedAt: now, Reason: "保证金不足强制平仓"}
		m.meta.NextOrder++
		m.orders[o.ID] = o
		if err := m.fill(a, s, o, now, true); err != nil {
			return err
		}
	}
	return nil
}
func (m *mutation) auction(now int64) error {
	if err := m.advanceSeason(now); err != nil {
		return err
	}
	m.cleanPending(now)
	// 开赛时只检查一次最新且仍有效的行情；历史补传不会触发后续小时的退市重放。
	season := m.meta.Season
	shanghai, _ := MarketLocation("Asia/Shanghai")
	if m.meta.DelistCheckedStart != season.StartsAt && !season.Settled && now >= season.StartsAt && now < season.EndsAt && time.UnixMilli(now).In(shanghai).Day() >= 5 {
		ttl := effectiveRules(m.meta.Settings).QuoteTTLSeconds * 1000
		for id := range m.e.stocks {
			stock := m.readStock(id)
			if stock.Delisted || stock.ObservedAt <= 0 || now-stock.ObservedAt > ttl || stock.Price >= m.meta.Settings.DelistThreshold*100 {
				continue
			}
			if err := m.delistStock(m.stock(id), now); err != nil {
				return err
			}
		}
		m.meta.DelistCheckedStart = season.StartsAt
		m.changed = true
	}
	hour := now / 3600000
	open, _ := m.marketOpen(now)
	if m.meta.AuctionHour >= hour && m.meta.AuctionOpen == open {
		return nil
	}
	m.meta.AuctionHour = hour
	if m.meta.AuctionOpen != open {
		m.marketChanged = true
	}
	m.meta.AuctionOpen = open
	m.changed = true
	for id, old := range m.e.stocks {
		if old.Hour != hour && (old.NormalUsed > 0 || old.BorrowUsed > 0) {
			s := m.stock(id)
			m.refreshHour(s, now)
		}
	}
	for sid, owners := range m.e.shortOwners {
		for accountID := range owners {
			a := m.account(accountID)
			if err := m.accrue(a, sid, now); err != nil {
				return err
			}
		}
	}
	for sid, owners := range m.e.longOwners {
		for accountID := range owners {
			a := m.account(accountID)
			if err := m.riskLong(a, m.stock(sid), now); err != nil {
				return err
			}
		}
	}
	for id, old := range m.e.stocks {
		hasShort := old.Borrowed > 0
		if s := m.stocks[id]; s != nil {
			old = s
		}
		if hasShort {
			if err := m.liquidateStock(m.stock(id), now); err != nil {
				return err
			}
		}
	}
	pending := []*Order{}
	if !open {
		return nil
	}
	for id := range m.e.pending {
		old := m.e.orders[id]
		o := old
		if fresh := m.orders[id]; fresh != nil {
			o = fresh
		}
		if o.Status == "pending" && o.EligibleHour <= hour {
			pending = append(pending, o)
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		if pending[i].Price == pending[j].Price {
			return pending[i].ID < pending[j].ID
		}
		return pending[i].Price > pending[j].Price
	})
	for _, old := range pending {
		o := m.order(old.ID)
		a := m.account(o.AccountID)
		s := m.stock(o.StockID)
		if a == nil || s == nil {
			continue
		}
		m.refreshHour(s, now)
		if err := m.fill(a, s, o, now, false); err != nil {
			if publicError(err).Code == "INSUFFICIENT_CASH" {
				m.cancel(a, o, now, "现金不足以支付成交费用")
				continue
			}
			return err
		}
		o.EligibleHour = hour + 1
	}
	return nil
}
func publicErrorOptionalCode(err error) string {
	if err == nil {
		return ""
	}
	return publicError(err).Code
}
