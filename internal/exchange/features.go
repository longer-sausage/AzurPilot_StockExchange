package exchange

import (
	"math/big"
	"sort"
	"strings"
	"time"
)

// Revenue 与资金账本一样以整数分计量，跨赛季累计独立于当月重置。
type Revenue struct {
	Tax        int64 `json:"tax"`
	Commission int64 `json:"commission"`
	Stamp      int64 `json:"stamp"`
	Levy       int64 `json:"levy"`
	Borrow     int64 `json:"borrow"`
	Financing  int64 `json:"financing"`
}
type Trade struct {
	ID        int64   `json:"id"`
	OrderID   int64   `json:"orderId"`
	StockID   int64   `json:"stockId"`
	AccountID int64   `json:"accountId"`
	Username  string  `json:"username"`
	Side      string  `json:"side"`
	Kind      string  `json:"kind"`
	Quantity  int64   `json:"quantity"`
	Price     int64   `json:"price"`
	Notional  int64   `json:"notional"`
	At        int64   `json:"at"`
	Forced    bool    `json:"forced"`
	Reason    string  `json:"reason,omitempty"`
	Fees      Revenue `json:"fees"`
}

func mergeRevenue(a *Revenue, b Revenue) error {
	for _, pair := range [][2]*int64{{&a.Tax, &b.Tax}, {&a.Commission, &b.Commission}, {&a.Stamp, &b.Stamp}, {&a.Levy, &b.Levy}, {&a.Borrow, &b.Borrow}, {&a.Financing, &b.Financing}} {
		if *pair[1] < 0 || *pair[0] > LedgerLimit-*pair[1] {
			return fail("NUMERICAL_LIMIT", "累计费用超出整数安全范围")
		}
		*pair[0] += *pair[1]
	}
	return nil
}
func (m *mutation) recordFees(a *Account, fee Revenue) error {
	if err := mergeRevenue(&a.Fees, fee); err != nil {
		return err
	}
	if err := mergeRevenue(&m.meta.Revenue, fee); err != nil {
		return err
	}
	return mergeRevenue(&m.meta.LifetimeRevenue, fee)
}
func accountReady(a *Account, lookup func(int64) *Stock) error {
	if a == nil || a.Disabled {
		return fail("UNAUTHORIZED", "账户不可用")
	}
	if a.ForcePasswordChange {
		return fail("PASSWORD_CHANGE_REQUIRED", "请先登录网页设置自己的新密码")
	}
	s := lookup(a.BoundStockID)
	if s == nil || s.DeletedAt != 0 || s.AccountID != a.ID {
		return fail("BINDING_REQUIRED", "交易账户必须先绑定一只股票才能使用，请向机器人发送 绑定股票 <绑定码>")
	}
	return nil
}
func (e *Engine) Ready(id int64) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return accountReady(e.accounts[id], func(sid int64) *Stock { return e.stocks[sid] })
}
func returnPPM(value, principal int64) int64 {
	if principal <= 0 {
		return 0
	}
	n := new(big.Int).Mul(big.NewInt(value-principal), big.NewInt(1_000_000))
	n.Quo(n, big.NewInt(principal))
	if !n.IsInt64() {
		return 0
	}
	return n.Int64()
}
func rankings(accounts map[int64]*Account, lookup func(int64) *Stock, cutoff int64) []SeasonRank {
	out := []SeasonRank{}
	for _, a := range accounts {
		if a.Disabled || a.BoundStockID == 0 || a.CreatedAt >= cutoff {
			continue
		}
		s := lookup(a.BoundStockID)
		if s == nil || s.DeletedAt != 0 || s.AccountID != a.ID {
			continue
		}
		value := equity(a, lookup)
		out = append(out, SeasonRank{AccountID: a.ID, Username: a.Username, Equity: value, ReturnPPM: returnPPM(value, a.InitialCash)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Equity == out[j].Equity {
			return out[i].AccountID < out[j].AccountID
		}
		return out[i].Equity > out[j].Equity
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}
func (e *Engine) Seasons() map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := []*SeasonArchive{}
	for _, s := range e.archives {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Season.ID > out[j].Season.ID })
	ranks := rankings(e.accounts, func(id int64) *Stock { return e.stocks[id] }, e.meta.Season.EndsAt)
	if archive := e.archives[e.meta.Season.ID]; archive != nil {
		ranks = archive.Rankings
	}
	return map[string]any{"season": e.meta.Season, "rankings": ranks, "archives": out, "revenue": e.meta.Revenue, "lifetimeRevenue": e.meta.LifetimeRevenue, "serverTime": e.clock().UnixMilli()}
}
func (e *Engine) SetWatchlist(id, sid int64, enabled bool) error {
	return e.transaction(func(m *mutation) error {
		if err := accountReady(m.readAccount(id), m.readStock); err != nil {
			return err
		}
		s := m.readStock(sid)
		if s == nil || s.DeletedAt != 0 {
			return fail("NOT_FOUND", "股票不存在")
		}
		old := m.readAccount(id)
		found := false
		for _, v := range old.Watchlist {
			if v == sid {
				found = true
			}
		}
		if found == enabled {
			return nil
		}
		a := m.account(id)
		a.Watchlist = nil
		for _, v := range old.Watchlist {
			if v != sid {
				a.Watchlist = append(a.Watchlist, v)
			}
		}
		if enabled {
			a.Watchlist = append(a.Watchlist, sid)
		}
		return nil
	})
}
func (e *Engine) DeleteStock(id int64) error {
	return e.transaction(func(m *mutation) error {
		s := m.readStock(id)
		if s == nil {
			return fail("NOT_FOUND", "股票不存在")
		}
		if s.DeletedAt != 0 {
			return nil
		}
		s = m.stock(id)
		now := e.clock().UnixMilli()
		owner := s.AccountID
		for oid := range e.pending {
			old := e.orders[oid]
			if old.StockID == id || old.AccountID == owner {
				o := m.order(oid)
				if o.Status == "pending" {
					m.cancel(m.account(o.AccountID), o, now, "管理员删除股票，撤销委托")
				}
			}
		}
		ids := []int64{}
		for aid, a := range e.accounts {
			if p := a.Positions[id]; p != nil && (p.Quantity > 0 || p.ShortQuantity > 0) {
				ids = append(ids, aid)
			}
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, aid := range ids {
			if err := m.closePosition(m.account(aid), s, now, "管理员删除股票，按最后报价结清"); err != nil {
				return err
			}
		}
		if owner != 0 {
			a := m.account(owner)
			if a != nil {
				a.BoundStockID = 0
			}
		}
		// 保留实例和公钥墓碑，阻止旧实例重新登记、继续上传及同名继承。
		s.AccountID = 0
		s.BindingCode = ""
		s.Disabled = true
		s.DeletedAt = now
		for aid, a := range e.accounts {
			for _, sid := range a.Watchlist {
				if sid == id {
					fresh := m.account(aid)
					kept := fresh.Watchlist[:0]
					for _, v := range fresh.Watchlist {
						if v != id {
							kept = append(kept, v)
						}
					}
					fresh.Watchlist = kept
					break
				}
			}
		}
		return nil
	})
}
func (e *Engine) StockData(id, from, to int64) (map[string]any, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s := e.stocks[id]
	if s == nil || s.DeletedAt != 0 {
		return nil, fail("NOT_FOUND", "股票不存在")
	}
	trades := []*Trade{}
	for _, tid := range e.stockTrades[id] {
		t := e.trades[tid]
		if t.At >= from && t.At <= to {
			trades = append(trades, t)
		}
	}
	sort.Slice(trades, func(i, j int) bool {
		if trades[i].At == trades[j].At {
			return trades[i].ID > trades[j].ID
		}
		return trades[i].At > trades[j].At
	})
	orders := []map[string]any{}
	legacy := []map[string]any{}
	for _, oids := range e.accountOrders {
		for _, oid := range oids {
			o := e.orders[oid]
			if o.StockID != id {
				continue
			}
			if o.Status == "pending" {
				v := publicOrder(o)
				delete(v, "clientId")
				v["accountId"] = o.AccountID
				v["username"] = e.accounts[o.AccountID].Username
				orders = append(orders, v)
			}
			if o.LegacyFilled > 0 && o.LegacyUpdatedAt >= from && o.LegacyUpdatedAt <= to {
				v := publicOrder(o)
				delete(v, "clientId")
				v["username"] = e.accounts[o.AccountID].Username
				v["filled"], v["notional"], v["averagePrice"], v["updatedAt"] = o.LegacyFilled, o.LegacyNotional, o.LegacyNotional/o.LegacyFilled, o.LegacyUpdatedAt
				legacy = append(legacy, v)
			}
		}
	}
	sort.Slice(orders, func(i, j int) bool { return orders[i]["id"].(int64) < orders[j]["id"].(int64) })
	return map[string]any{"stock": publicStock(s), "trades": trades, "orders": orders, "legacyOrders": legacy}, nil
}
func shanghaiDay(ms int64) string {
	return time.UnixMilli(ms).In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
}
func (e *Engine) WatchAccount(id int64) (map[string]any, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	a := e.accounts[id]
	if a == nil {
		return nil, fail("NOT_FOUND", "账户不存在")
	}
	return accountPublic(a, func(sid int64) *Stock { return e.stocks[sid] }), nil
}
func validName(name string) bool {
	return len([]rune(strings.TrimSpace(name))) >= 2 && len([]rune(name)) <= 32 && !strings.ContainsAny(name, "\r\n\x00")
}
func (m *mutation) cleanPending(now int64) {
	for id := range m.e.pending {
		o := m.e.orders[id]
		if fresh := m.orders[id]; fresh != nil {
			o = fresh
		}
		if o.Status != "pending" {
			continue
		}
		a := m.readAccount(o.AccountID)
		s := m.readStock(o.StockID)
		if immediateOrder(o) {
			m.cancel(m.account(a.ID), m.order(id), now, "市价委托未成交余量自动撤销")
			continue
		}
		if o.ExpiresAt > 0 && now >= o.ExpiresAt {
			fresh := m.order(id)
			m.cancel(m.account(a.ID), fresh, now, "DAY 委托到期")
			fresh.Status = "expired"
			continue
		}
		if a.Disabled || a.BoundStockID == 0 || s == nil || s.DeletedAt != 0 || s.Disabled {
			m.cancel(m.account(a.ID), m.order(id), now, "账户未绑定、已停用或股票不可交易")
		}
	}
}
func (e *Engine) migrateFeatures() error {
	if e.meta.FeatureVersion >= 1 && len(e.retiredRegistrationIDs) == 0 {
		return nil
	}
	return e.transaction(func(m *mutation) error {
		m.removedRegistrations = append(m.removedRegistrations, e.retiredRegistrationIDs...)
		if m.meta.FeatureVersion >= 1 {
			m.changed = len(m.removedRegistrations) > 0
			return nil
		}
		for id, old := range e.accounts {
			a := m.account(id)
			a.MetricsSince = e.clock().UnixMilli()
			a.LegacyMetrics = true
			if a.IdentityCode == "" {
				a.IdentityCode = "MMEX-" + randomToken(18)
			}
			for _, p := range a.Positions {
				if p.ShortQuantity > 0 && p.ShortCost == 0 {
					p.ShortCost = p.ShortProceeds
				}
			}
			for _, oid := range e.accountOrders[old.ID] {
				o := e.orders[oid]
				if o.CreatedAt < m.meta.Season.StartsAt || o.CreatedAt >= m.meta.Season.EndsAt {
					continue
				}
				if err := mergeRevenue(&a.Fees, Revenue{Tax: o.Tax, Commission: o.Commission, Stamp: o.Stamp, Levy: o.Levy}); err != nil {
					return err
				}
			}
		}
		for id, old := range e.orders {
			o := m.order(id)
			if o.Kind == "" {
				o.Kind = "limit"
			}
			if o.TimeInForce == "" {
				o.TimeInForce = "GTC"
			}
			if !old.TradeRecorded && old.Filled > 0 {
				o.LegacyFilled, o.LegacyNotional, o.LegacyUpdatedAt = old.Filled, old.Notional, old.UpdatedAt
			}
			fee := Revenue{Tax: o.Tax, Commission: o.Commission, Stamp: o.Stamp, Levy: o.Levy}
			if err := mergeRevenue(&m.meta.LifetimeRevenue, fee); err != nil {
				return err
			}
			if o.CreatedAt >= m.meta.Season.StartsAt && o.CreatedAt < m.meta.Season.EndsAt {
				if err := mergeRevenue(&m.meta.Revenue, fee); err != nil {
					return err
				}
			}
		}

		for id, s := range e.stocks {
			if s.DeletedAt == 0 {
				if err := m.updateOpening(m.stock(id), nil); err != nil {
					return err
				}
			}
		}
		m.cleanPending(e.clock().UnixMilli())
		m.meta.FeatureVersion = 1
		m.changed = true
		return nil
	})
}
func accountMetrics(a *Account, lookup func(int64) *Stock, settings Settings, now int64) map[string]int64 {
	longPnL, shortPnL, loans, liability, collateral, proceeds, maintenance, initial, unsettled, financing, borrow := int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0)
	rules := effectiveRules(settings)
	for id, p := range a.Positions {
		s := lookup(id)
		if s == nil {
			continue
		}
		value := p.Quantity * s.Price
		short := p.ShortQuantity * s.Price
		longPnL += value - p.Cost
		shortPnL += p.ShortCost - short
		loans += p.Loan
		liability += short
		collateral += p.Collateral
		proceeds += p.ShortProceeds
		if p.Loan > 0 {
			maintenance += rate(value, rules.MMRPPM)
			initial += rate(value, rules.IMRPPM)
			fee, _ := carriedInterest(p.Loan, rules.FinancingAnnualPPM, max(int64(0), now-p.FinancingAt), p.FinancingRemainder)
			financing += fee
		}
		if p.ShortQuantity > 0 {
			maintenance += rate(short, rules.MMRPPM)
			initial += rate(short, rules.IMRPPM)
			fee, _ := carriedInterest(short, rules.BorrowAnnualPPM, max(int64(0), now-p.BorrowAt), p.BorrowRemainder)
			borrow += fee
		}
	}
	for _, v := range a.Settlements {
		if v.AvailableAt > now {
			unsettled += v.Amount
		}
	}
	return map[string]int64{"longPnL": longPnL, "shortPnL": shortPnL, "floatingPnL": longPnL + shortPnL, "financingDebt": loans, "shortLiability": liability, "collateral": collateral, "shortProceeds": proceeds, "initialMargin": initial, "maintenanceMargin": maintenance, "unsettled": unsettled, "financingAccrued": financing, "borrowAccrued": borrow, "orderFrozen": a.Reserved}
}

func (e *Engine) indexTrade(t *Trade) {
	ids := e.stockTrades[t.StockID]
	at := sort.Search(len(ids), func(i int) bool { old := e.trades[ids[i]]; return old.At > t.At || old.At == t.At && old.ID > t.ID })
	ids = append(ids, 0)
	copy(ids[at+1:], ids[at:])
	ids[at] = t.ID
	e.stockTrades[t.StockID] = ids
}
