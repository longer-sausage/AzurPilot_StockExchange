package exchange

import (
	"database/sql"
	"encoding/json"
	"time"
)

func (e *Engine) Submit(id int64, in OrderInput) (*Order, error) {
	if !clientIDPattern.MatchString(in.ClientID) {
		return nil, fail("INVALID_ORDER", "委托幂等标识无效")
	}
	var result *Order
	err := e.transaction(func() error {
		now := e.clock()
		if err := e.advance(now); err != nil {
			return err
		}
		p := e.players[id]
		if p == nil || p.Disabled {
			return fail("UNAUTHORIZED", "账户不可用")
		}
		for _, o := range p.Orders {
			if o.ClientID == in.ClientID {
				copy := *o
				result = &copy
				return nil
			}
		}
		var raw string
		err := e.db.QueryRow("SELECT data FROM receipts WHERE player_id=? AND client_id=?", id, in.ClientID).Scan(&raw)
		if err == nil {
			var o Order
			if err = json.Unmarshal([]byte(raw), &o); err != nil {
				return err
			}
			result = &o
			return nil
		}
		if err != sql.ErrNoRows {
			return err
		}
		if open, reason := e.isOpen(now); !open {
			return fail("MARKET_CLOSED", reason)
		}
		if in.StockID == id {
			return fail("SELF_TRADE", "不能交易自己的股票，避免利用自报股价套利")
		}
		stock := e.players[in.StockID]
		if stock == nil || stock.Disabled {
			return fail("NOT_FOUND", "该股票不存在或已停牌")
		}
		if stock.Delisted {
			return fail("STOCK_DELISTED", "该股票本月已退市，下月重新判断上市资格")
		}
		r := e.state.Settings.Active
		if !e.quoteFresh(stock, now.Unix()) {
			return fail("STALE_QUOTE", "该股票行动力报价已过期，等待持有人同步")
		}
		if in.Side != "buy" && in.Side != "sell" && in.Side != "short" && in.Side != "cover" {
			return fail("INVALID_ORDER", "委托方向无效")
		}
		if err := validateLeverage(in.Side, in.Leverage, r); err != nil {
			return err
		}
		if in.Kind != "market" && in.Kind != "limit" && in.Kind != "stop" {
			return fail("INVALID_ORDER", "委托类型无效")
		}
		if in.TIF != "DAY" && in.TIF != "GTC" {
			return fail("INVALID_ORDER", "有效期需为 DAY 或 GTC")
		}
		if in.Quantity < 1 || in.Quantity > 1_000_000 || in.Quantity%r.LotSize != 0 {
			return fail("INVALID_ORDER", "数量须符合整手单位，最大 1,000,000 股")
		}
		if in.Kind != "market" && (in.Limit < 1 || in.Limit > 100_000_000) {
			return fail("INVALID_ORDER", "委托价格须在 0.01–1,000,000 之间")
		}
		if stock.Quote.Price*in.Quantity > MaxNotional || in.Limit*in.Quantity > MaxNotional {
			return fail("INVALID_ORDER", "委托金额超出限制")
		}
		count := 0
		for _, o := range p.Orders {
			if o.Status == "pending" {
				count++
			}
		}
		if count >= 20 {
			return fail("TOO_MANY_ORDERS", "每个账户最多同时挂 20 笔委托")
		}
		p = e.touch(id)
		e.accrue(p, now.Unix())
		o := &Order{ID: e.state.NextOrder, ClientID: in.ClientID, StockID: in.StockID, Side: in.Side, Kind: in.Kind, TIF: in.TIF, Quantity: in.Quantity, Limit: in.Limit, Leverage: in.Leverage, Status: "pending", CreatedAt: now.Unix(), Rules: r}
		e.state.NextOrder++
		if in.TIF == "DAY" {
			l, _ := loadLocation(r.Timezone)
			t := now.In(l)
			o.ExpiresAt = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, l).Unix()
		}
		if err := e.validatePosition(p, o, now.Unix()); err != nil {
			return err
		}
		price := stock.Quote.Price
		if o.Kind != "market" {
			price = o.Limit
		}
		amount := price * o.Quantity
		fees := FeesFor(r, o.Side, amount).Total()
		if o.Side == "buy" {
			o.Reserved = rate(amount, orderMargin(o, r)) + fees
		}
		if o.Side == "short" {
			o.Reserved = rate(amount, orderMargin(o, r)) + fees
		}
		if o.Reserved > e.account(p, now.Unix()).Available {
			return fail("INSUFFICIENT_CASH", "可用资金不足（含费用、保证金与挂单冻结）")
		}
		p.Orders = append(p.Orders, o)
		if e.triggered(o, stock.Quote.Price) {
			if err := e.fill(p, o, stock.Quote.Price, now, false); err != nil {
				return err
			}
		}
		trimOrders(p)
		copy := *o
		result = &copy
		return nil
	})
	return result, err
}
func trimOrders(p *Player) {
	if len(p.Orders) <= 120 {
		return
	}
	keep := []*Order{}
	completed := 0
	for i := len(p.Orders) - 1; i >= 0; i-- {
		o := p.Orders[i]
		if o.Status == "pending" || completed < 100 {
			keep = append(keep, o)
			if o.Status != "pending" {
				completed++
			}
		}
	}
	for i, j := 0, len(keep)-1; i < j; i, j = i+1, j-1 {
		keep[i], keep[j] = keep[j], keep[i]
	}
	p.Orders = keep
}
func (e *Engine) validatePosition(p *Player, o *Order, now int64) error {
	if o.Side == "buy" || o.Side == "short" {
		if p.Cash > LedgerSafetyLimit || p.Cash < -LedgerSafetyLimit {
			return fail("NUMERICAL_LIMIT", "账户已到整数账本安全边界，请先平仓")
		}
		remaining := LedgerSafetyLimit / MaxQuotePrice
		consume := func(q int64) bool {
			if q < 0 {
				q = -q
			}
			if q < 0 || q > remaining {
				return false
			}
			remaining -= q
			return true
		}
		for _, v := range p.Positions {
			if !consume(v.Quantity) {
				return fail("NUMERICAL_LIMIT", "最坏报价下的持仓超出整数账本安全边界")
			}
		}
		for _, v := range p.Orders {
			if v.ID != o.ID && v.Status == "pending" && (v.Side == "buy" || v.Side == "short") && !consume(v.Quantity) {
				return fail("NUMERICAL_LIMIT", "持仓与开仓委托超出整数账本安全边界")
			}
		}
		if !consume(o.Quantity) {
			return fail("NUMERICAL_LIMIT", "新增持仓超出整数账本安全边界")
		}
	}
	pos := p.Positions[o.StockID]
	qty := int64(0)
	if pos != nil {
		qty = pos.Quantity
	}
	if (o.Side == "buy" && qty < 0) || (o.Side == "short" && qty > 0) {
		return fail("OPPOSITE_POSITION", "先平掉相反方向的持仓，再开新仓")
	}
	for _, v := range p.Orders {
		if v.ID == o.ID || v.StockID != o.StockID || v.Status != "pending" {
			continue
		}
		if (o.Side == "buy" && v.Side == "short") || (o.Side == "short" && v.Side == "buy") {
			return fail("OPPOSITE_POSITION", "已有相反方向的开仓委托，请先撤单")
		}
	}
	if o.Side == "sell" {
		avail := int64(0)
		if pos != nil && qty > 0 {
			for _, lot := range pos.Lots {
				if lot.AvailableAt <= now {
					avail += lot.Quantity
				}
			}
		}
		for _, v := range p.Orders {
			if v.ID != o.ID && v.StockID == o.StockID && v.Side == "sell" && v.Status == "pending" {
				avail -= v.Quantity
			}
		}
		if o.Quantity > avail {
			return fail("INSUFFICIENT_SHARES", "可卖股份不足，可能受 T+N 或已有卖单限制")
		}
	}
	if o.Side == "cover" {
		avail := -min(qty, 0)
		for _, v := range p.Orders {
			if v.ID != o.ID && v.StockID == o.StockID && v.Side == "cover" && v.Status == "pending" {
				avail -= v.Quantity
			}
		}
		if o.Quantity > avail {
			return fail("INSUFFICIENT_SHORT", "可平空仓不足，检查已有回补委托")
		}
	}
	return nil
}
func (e *Engine) triggered(o *Order, price int64) bool {
	if o.Kind == "market" {
		return true
	}
	buy := o.Side == "buy" || o.Side == "cover"
	if o.Kind == "limit" {
		if buy {
			return price <= o.Limit
		}
		return price >= o.Limit
	}
	if buy {
		return price >= o.Limit
	}
	return price <= o.Limit
}
func (e *Engine) fill(p *Player, o *Order, price int64, now time.Time, forced bool) error {
	if !forced {
		if err := validateLeverage(o.Side, o.Leverage, e.state.Settings.Active); err != nil {
			return err
		}
		if err := e.validatePosition(p, o, now.Unix()); err != nil {
			return err
		}
	}
	amount := price * o.Quantity
	fees := FeesFor(o.Rules, o.Side, amount)
	a := e.account(p, now.Unix())
	available := a.Available + o.Reserved
	own := rate(amount, orderMargin(o, e.state.Settings.Active))
	if !forced && o.Side == "buy" && own+fees.Total() > available {
		return fail("INSUFFICIENT_CASH", "触发时可用资金不足，委托无法成交")
	}
	if !forced && o.Side == "short" && own+fees.Total() > available {
		return fail("INSUFFICIENT_MARGIN", "触发时做空保证金不足")
	}
	pos := p.Positions[o.StockID]
	if pos == nil {
		pos = &Position{StockID: o.StockID, Lots: []Lot{}}
		p.Positions[o.StockID] = pos
	}
	switch o.Side {
	case "buy":
		addMargin(pos, o.Quantity, orderMargin(o, e.state.Settings.Active), e.state.Settings.Active)
		loan := amount - own
		p.Cash -= own
		pos.Loan += loan
		if pos.Loan > 0 {
			pos.FinancingAt = now.Unix()
		}
		pos.Quantity += o.Quantity
		pos.Cost += amount + fees.Total()
		pos.Lots = append(pos.Lots, Lot{o.Quantity, availableAt(now, o.Rules.SellDelayDays, o.Rules)})
	case "sell":
		reduceMargin(pos, o.Quantity, e.state.Settings.Active)
		repayment := proportional(pos.Loan, o.Quantity, pos.Quantity)
		pos.Loan -= repayment
		cost := pos.Cost/pos.Quantity*o.Quantity + pos.Cost%pos.Quantity*o.Quantity/pos.Quantity
		pos.Cost -= cost
		pos.Quantity -= o.Quantity
		p.Cash += amount - repayment
		p.Realized += amount - fees.Total() - cost
		left := o.Quantity
		for i := range pos.Lots {
			lot := &pos.Lots[i]
			if lot.AvailableAt <= now.Unix() || forced {
				take := min(left, lot.Quantity)
				lot.Quantity -= take
				left -= take
			}
		}
		lots := []Lot{}
		for _, lot := range pos.Lots {
			if lot.Quantity > 0 {
				lots = append(lots, lot)
			}
		}
		pos.Lots = lots
		if o.Rules.CashDelayDays > 0 && !forced {
			p.Settlements = append(p.Settlements, Settlement{max(int64(0), amount-repayment-fees.Total()), availableAt(now, o.Rules.CashDelayDays, o.Rules)})
		}
	case "short":
		addMargin(pos, o.Quantity, orderMargin(o, e.state.Settings.Active), e.state.Settings.Active)
		p.Cash += amount
		pos.Quantity -= o.Quantity
		pos.Cost += amount - fees.Total()
		pos.BorrowAt = now.Unix()
	case "cover":
		if !forced && p.Cash-amount-fees.Total() < a.Unsettled {
			return fail("INSUFFICIENT_CASH", "回补资金不足，先卖出多头持仓或等待交收")
		}
		reduceMargin(pos, o.Quantity, e.state.Settings.Active)
		cost := pos.Cost/(-pos.Quantity)*o.Quantity + pos.Cost%(-pos.Quantity)*o.Quantity/(-pos.Quantity)
		pos.Cost -= cost
		pos.Quantity += o.Quantity
		p.Cash -= amount
		p.Realized += cost - amount - fees.Total()
	}
	p.Cash -= fees.Total()
	p.Fees.Add(fees)
	e.state.Season.Revenue.Add(fees)
	e.state.LifetimeRevenue.Add(fees)
	o.Status = "filled"
	o.ExecutedAt = now.Unix()
	o.Price = price
	o.Fees = fees
	o.Reserved = 0
	e.trades = append(e.trades, Trade{ID: o.ID, StockID: o.StockID, Username: p.Username, Side: o.Side, Kind: o.Kind, Quantity: o.Quantity, Price: price, Time: now.UnixMilli(), Forced: forced})
	if pos.Quantity == 0 {
		delete(p.Positions, o.StockID)
	}
	return nil
}
func (e *Engine) processOrders(p *Player, now time.Time) {
	open, _ := e.isOpen(now)
	for _, o := range p.Orders {
		if o.Status != "pending" {
			continue
		}
		if o.ExpiresAt > 0 && now.Unix() >= o.ExpiresAt {
			o.Status = "expired"
			o.Reserved = 0
			o.Reason = "DAY 委托已到期"
			continue
		}
		stock := e.players[o.StockID]
		if stock == nil || stock.Disabled || stock.Delisted {
			o.Status = "cancelled"
			o.Reserved = 0
			o.Reason = "股票已停牌"
			if stock != nil && stock.Delisted {
				o.Reason = "股票本月已退市"
			}
			continue
		}
		if !open || !e.quoteFresh(stock, now.Unix()) || !e.triggered(o, stock.Quote.Price) {
			continue
		}
		if err := e.fill(p, o, stock.Quote.Price, now, false); err != nil {
			o.Status = "rejected"
			o.Reserved = 0
			o.Reason = err.Error()
		}
	}
	trimOrders(p)
}
func cancelPending(p *Player, reason string) {
	for _, o := range p.Orders {
		if o.Status == "pending" {
			o.Status = "cancelled"
			o.Reserved = 0
			o.Reason = reason
		}
	}
}
func (e *Engine) forcedClose(p *Player, now time.Time, reason string) {
	cancelPending(p, reason)
	// 先卖多头，再回补空头；风控和月末结算不受休市及 T+N 约束。
	for _, side := range []string{"sell", "cover"} {
		for stock, pos := range p.Positions {
			if (side == "sell" && pos.Quantity > 0) || (side == "cover" && pos.Quantity < 0) {
				e.forcedCloseStock(p, stock, now, reason)
			}
		}
	}
	trimOrders(p)
}
func (e *Engine) forcedCloseStock(p *Player, stock int64, now time.Time, reason string) {
	pos := p.Positions[stock]
	if pos == nil || pos.Quantity == 0 {
		return
	}
	qty, side := pos.Quantity, "sell"
	if qty < 0 {
		qty, side = -qty, "cover"
	}
	o := &Order{ID: e.state.NextOrder, ClientID: "system_" + symbol(e.state.NextOrder), StockID: stock, Side: side, Kind: "market", TIF: "DAY", Quantity: qty, CreatedAt: now.Unix(), Status: "pending", Reason: reason, Rules: e.state.Settings.Active}
	e.state.NextOrder++
	_ = e.fill(p, o, e.players[stock].Quote.Price, now, true)
	p.Orders = append(p.Orders, o)
}
func (e *Engine) risk(p *Player, now time.Time) {
	a := e.account(p, now.Unix())
	if (a.ShortLiability > 0 || a.FinancingDebt > 0) && a.Equity < a.MaintenanceMargin {
		e.forcedClose(p, now, "维持保证金不足，系统强制平仓")
	}
}
func (e *Engine) Cancel(id, orderID int64) error {
	return e.transaction(func() error {
		if err := e.advance(e.clock()); err != nil {
			return err
		}
		p := e.players[id]
		if p == nil {
			return fail("NOT_FOUND", "账户不存在")
		}
		p = e.touch(id)
		for _, o := range p.Orders {
			if o.ID == orderID {
				if o.Status != "pending" {
					return fail("ORDER_FINAL", "委托已成交或已结束")
				}
				o.Status = "cancelled"
				o.Reserved = 0
				return nil
			}
		}
		return fail("NOT_FOUND", "委托不存在")
	})
}
func (e *Engine) Disable(id int64, disabled bool) error {
	_, err := e.UpdatePlayer(id, AdminPlayerUpdate{Disabled: &disabled})
	return err
}
