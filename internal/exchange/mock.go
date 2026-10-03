package exchange

import (
	"fmt"
	"math"
	"time"
)

// mock 复用真实交易引擎；只有验证码、种子行情和赛季时间使用模拟数据。
func SeedMock(e *Engine) error {
	now := e.clock()
	if len(e.players) == 0 {
		for i, name := range []string{"茗喵", "海风指挥官", "皇家方舟", "星海旅人", "白鹰后勤", "港区观察员", "摸鱼猫猫", "北方联合"} {
			_, _, err := e.Register(name, "mock-player-password", int64(2680+i*730), now.Unix())
			if err != nil {
				return err
			}
		}
	}
	return e.transaction(func() error {
		e.state.Season.StartsAt = now.Add(-48 * time.Hour).Unix()
		e.state.Season.EndsAt = now.Add(25 * 24 * time.Hour).Unix()
		e.state.Season.Settled = false
		for id := range e.players {
			if id > 8 {
				continue
			}
			p := e.touch(id)
			p.Quote.ObservedAt = now.Unix()
			p.Quote.UploadedAt = now.Unix() - 60
			p.Quote.Previous = p.Quote.Price * 100 / 103
			var count int
			if err := e.db.QueryRow("SELECT COUNT(*) FROM minute_prices WHERE stock_id=?", id).Scan(&count); err != nil {
				return err
			}
			if count < 1000 {
				points := []HistoryPoint{}
				// 45 天的稀疏记录与最近两天的密集记录，展示真实聚合方式和指标。
				for minute := 45 * 24 * 60; minute >= 0; {
					t := now.Add(-time.Duration(minute) * time.Minute).UnixMilli()
					base := p.Quote.Price / 100
					offset := int64(float64(base) * (.028*math.Sin(float64(minute)/133) + .012*math.Sin(float64(minute)/27)))
					ap := max(int64(1), base+offset)
					points = append(points, HistoryPoint{t - 15000, max(int64(1), ap-int64(minute%17))}, HistoryPoint{t, ap})
					if minute > 2*24*60 {
						minute -= 30
					} else {
						minute -= 2
					}
				}
				points[len(points)-1].ActionPoints = p.Quote.Price / 100
				e.histories = append(e.histories, historyWrite{ID: id, Report: HistoryReport{Points: points}, Provisional: true})
			}
			var trades int
			if err := e.db.QueryRow("SELECT COUNT(*) FROM trades WHERE stock_id=?", id).Scan(&trades); err != nil {
				return err
			}
			if trades == 0 {
				trader := e.touch(id%8 + 1)
				for j := 10; j >= 0; j-- {
					at := now.Add(-time.Duration(j*47) * time.Minute)
					qty := int64(12 + j*3)
					for _, side := range []string{"buy", "sell"} {
						o := &Order{ID: e.state.NextOrder, ClientID: fmt.Sprintf("mock_trade_%d", e.state.NextOrder), StockID: id, Side: side, Kind: "market", TIF: "GTC", Quantity: qty, Status: "pending", CreatedAt: at.Unix(), Rules: e.state.Settings.Active, Leverage: 1}
						e.state.NextOrder++
						if err := e.fill(trader, o, p.Quote.Price, at, false); err != nil {
							return err
						}
						trader.Orders = append(trader.Orders, o)
					}
				}
			}
		}
		return nil
	})
}
