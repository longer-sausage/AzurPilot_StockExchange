package exchange

import (
	"encoding/json"
	"fmt"
	"time"
)

func migrateDelistThreshold(raw string, settings *Settings) {
	var stored struct {
		Settings struct {
			DelistThreshold *int64 `json:"delistThreshold"`
		} `json:"settings"`
	}
	if json.Unmarshal([]byte(raw), &stored) == nil && stored.Settings.DelistThreshold == nil {
		settings.DelistThreshold = DefaultDelistThreshold
	}
}

func (e *Engine) belowDelistThreshold(p *Player) bool {
	return p != nil && !p.Disabled && !p.Delisted && p.Quote.Price < e.state.Settings.DelistThreshold*100
}

func (e *Engine) delistIfNeeded(stock *Player, now time.Time) {
	if !e.belowDelistThreshold(stock) || e.state.Season.Settled || now.Unix() < e.state.Season.StartsAt || now.Unix() >= e.state.Season.EndsAt {
		return
	}
	zone, _ := loadLocation("Asia/Shanghai")
	if now.In(zone).Day() < 5 {
		return
	}
	e.touch(stock.ID).Delisted = true
	reason := fmt.Sprintf("总行动力低于 %d，股票本月强制退市", e.state.Settings.DelistThreshold)
	// 退市只结算这支股票；账户仍可登录、上传和交易其他证券。
	for id := range e.watchers[stock.ID] {
		p := e.touch(id)
		e.accrue(p, now.Unix())
		for _, o := range p.Orders {
			if o.StockID == stock.ID && o.Status == "pending" {
				o.Status, o.Reserved, o.Reason = "cancelled", 0, reason
			}
		}
		e.forcedCloseStock(p, stock.ID, now, reason)
		trimOrders(p)
		e.risk(p, now)
	}
}
