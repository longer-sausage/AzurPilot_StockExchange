package exchange

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// 只在事务提交后唤醒订阅者，回滚不会向页面发布不存在的变更。
func (e *Engine) updates() (uint64, <-chan struct{}) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state.Revision, e.changes
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	controller := http.NewResponseController(w)
	// 长连接逐次刷新写入期限，避免继承普通请求的整体写超时。
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		revision, changed := s.engine.updates()
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := fmt.Fprintf(w, "event: stock\ndata: {\"revision\":%d,\"serverTime\":%d}\n\n", revision, s.engine.clock().Unix()); err != nil {
			return
		}
		if err := controller.Flush(); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-changed:
		case <-ticker.C:
		}
	}
}

func (e *Engine) SetWatchlist(id, stockID int64, selected bool) ([]int64, error) {
	var result []int64
	err := e.transaction(func() error {
		p := e.players[id]
		if p == nil || p.Disabled {
			return fail("UNAUTHORIZED", "账户不可用")
		}
		if e.players[stockID] == nil {
			return fail("NOT_FOUND", "证券不存在")
		}
		result = make([]int64, 0, len(p.Watchlist)+1)
		found := false
		for _, current := range p.Watchlist {
			if current == stockID {
				found = true
				if !selected {
					continue
				}
			}
			result = append(result, current)
		}
		if selected && !found {
			result = append(result, stockID)
		}
		if found != selected {
			e.touch(id).Watchlist = result
		}
		return nil
	})
	return result, err
}

func (s *Server) watchlist(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		StockID  int64 `json:"stockId"`
		Selected *bool `json:"selected"`
	}
	if err := decode(w, r, &in); err != nil {
		s.error(w, 400, err)
		return
	}
	if in.StockID <= 0 || in.Selected == nil {
		s.error(w, 400, fail("INVALID_PARAMS", "自选请求须包含证券标识和选中状态"))
		return
	}
	list, err := s.engine.SetWatchlist(id, in.StockID, *in.Selected)
	if err != nil {
		s.error(w, 400, err)
		return
	}
	s.json(w, 200, map[string]any{"watchlist": list})
}

// 老版本只在账户中保留部分订单，完整幂等回执仍保存在数据库。
func (e *Engine) orderHistory(id int64) ([]*Order, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	rows, err := e.db.Query("SELECT data FROM receipts WHERE player_id=? ORDER BY json_extract(data,'$.id')", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	orders := []*Order{}
	for rows.Next() {
		var raw string
		var order Order
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &order); err != nil {
			return nil, err
		}
		if order.Status == "pending" && order.CreatedAt < e.state.Season.StartsAt {
			order.Status, order.Reason = "expired", "所属月赛已结束"
			order.Reserved = 0
		}
		orders = append(orders, &order)
	}
	return orders, rows.Err()
}

func (s *Server) orders(w http.ResponseWriter, r *http.Request, id int64) {
	orders, err := s.engine.orderHistory(id)
	if err != nil {
		s.error(w, 500, err)
		return
	}
	s.json(w, 200, orders)
}
