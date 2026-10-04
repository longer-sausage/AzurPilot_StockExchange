package exchange

import "time"

func (e *Engine) advance(now time.Time) error {
	current := seasonFor(now, e.state.Settings)
	if !e.state.Season.Settled && (now.Unix() >= e.state.Season.EndsAt || current.ID != e.state.Season.ID) {
		cutoff := time.Unix(e.state.Season.EndsAt, 0)
		for id := range e.players {
			p := e.touch(id)
			e.accrue(p, cutoff.Unix())
			e.forcedClose(p, cutoff, "月赛截止，按最后有效行动力价格结算")
			p.Settlements = []Settlement{}
		}
		e.state.Season.Settled = true
		e.archives = append(e.archives, SeasonResult{e.state.Season, e.ranks(cutoff.Unix())})
	}
	if current.ID != e.state.Season.ID {
		e.state.Season = current
		for id := range e.players {
			p := e.touch(id)
			p.Cash = e.state.Settings.InitialCash
			p.InitialCash = e.state.Settings.InitialCash
			p.Positions = map[int64]*Position{}
			p.Settlements = []Settlement{}
			p.Orders = []*Order{}
			p.Fees = Fees{}
			p.Realized = 0
			p.Delisted = false
		}
		if now.Unix() >= current.EndsAt {
			e.state.Season.Settled = true
			e.archives = append(e.archives, SeasonResult{e.state.Season, e.ranks(now.Unix())})
		}
		// 长期离线跨月后，按新月份重新判断全部股票。
		for _, p := range e.players {
			e.delistIfNeeded(p, now)
		}
	}
	for id := range e.delistCandidates {
		e.delistIfNeeded(e.players[id], now)
	}
	return nil
}
func (e *Engine) Tick() error {
	return e.transaction(func() error {
		now := e.clock()
		if err := e.advance(now); err != nil {
			return err
		}
		if len(e.activePlayers) == 0 {
			return nil
		}
		open, _ := e.isOpen(now)
		for id := range e.activePlayers {
			p := e.players[id]
			needs := hasMargin(p) && !e.state.Season.Settled
			for _, o := range p.Orders {
				stock := e.players[o.StockID]
				if o.Status == "pending" && (o.ExpiresAt > 0 && now.Unix() >= o.ExpiresAt || stock == nil || stock.Disabled || stock.Delisted || open && e.quoteFresh(stock, now.Unix()) && e.triggered(o, stock.Quote.Price)) {
					needs = true
					break
				}
			}
			for _, v := range p.Settlements {
				if v.AvailableAt <= now.Unix() {
					needs = true
					break
				}
			}
			if !needs {
				continue
			}
			p = e.touch(id)
			e.accrue(p, now.Unix())
			settlements := []Settlement{}
			for _, v := range p.Settlements {
				if v.AvailableAt > now.Unix() {
					settlements = append(settlements, v)
				}
			}
			p.Settlements = settlements
			e.risk(p, now)
			e.processOrders(p, now)
		}
		return nil
	})
}
