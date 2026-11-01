package exchange

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

func orderKey(id int64, client string) string { return idstr(id) + ":" + client }
func (e *Engine) initializeIndexes() {
	e.byClient = map[string]int64{}
	e.byPreview = map[string]int64{}
	e.pending = map[int64]bool{}
	e.accountOrders = map[int64][]int64{}
	e.byOfficial = map[string]int64{}
	e.byUsername = map[string]int64{}
	e.byInstance = map[string]int64{}
	e.shortOwners = map[int64]map[int64]bool{}
	e.longOwners = map[int64]map[int64]bool{}
	for _, a := range e.accounts {
		e.indexAccount(a)
	}
	for _, s := range e.stocks {
		e.byInstance[s.InstanceID] = s.ID
	}
	for _, o := range e.orders {
		e.indexOrder(o, true)
	}
	for id := range e.accountOrders {
		sort.Slice(e.accountOrders[id], func(i, j int) bool { return e.accountOrders[id][i] < e.accountOrders[id][j] })
	}
}
func (e *Engine) indexAccount(a *Account) {
	if old := e.accounts[a.ID]; old != nil && normalized(old.Username) != normalized(a.Username) {
		delete(e.byUsername, normalized(old.Username))
	}
	e.byOfficial[a.OfficialUserID] = a.ID
	e.byUsername[normalized(a.Username)] = a.ID
	if old := e.accounts[a.ID]; old != nil {
		for sid := range old.Positions {
			if owners := e.longOwners[sid]; owners != nil {
				delete(owners, a.ID)
			}
			if owners := e.shortOwners[sid]; owners != nil {
				delete(owners, a.ID)
			}
		}
	}
	for sid, p := range a.Positions {
		if p.Loan > 0 {
			if e.longOwners[sid] == nil {
				e.longOwners[sid] = map[int64]bool{}
			}
			e.longOwners[sid][a.ID] = true
		}
		if p.ShortQuantity > 0 {
			if e.shortOwners[sid] == nil {
				e.shortOwners[sid] = map[int64]bool{}
			}
			e.shortOwners[sid][a.ID] = true
		}
	}
}
func (e *Engine) indexOrder(o *Order, newOrder bool) {
	if o.ClientID != "" {
		e.byClient[orderKey(o.AccountID, o.ClientID)] = o.ID
	}
	if o.PreviewID != "" {
		e.byPreview[orderKey(o.AccountID, o.PreviewID)] = o.ID
	}
	if o.Status == "pending" {
		e.pending[o.ID] = true
	} else {
		delete(e.pending, o.ID)
	}
	if newOrder {
		e.accountOrders[o.AccountID] = append(e.accountOrders[o.AccountID], o.ID)
		sort.Slice(e.accountOrders[o.AccountID], func(i, j int) bool { return e.accountOrders[o.AccountID][i] < e.accountOrders[o.AccountID][j] })
	}
}
func (e *Engine) PortfolioTag(id int64) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	a := e.accounts[id]
	if a == nil || a.Disabled {
		return "", fail("UNAUTHORIZED", "账户不可用")
	}
	versions := map[int64]uint64{}
	availableLots := map[int64]int64{}
	for sid, p := range a.Positions {
		availableLots[sid] = sellable(p, e.clock().UnixMilli())
		if p.Quantity != 0 || p.ShortQuantity != 0 {
			if s := e.stocks[sid]; s != nil {
				versions[sid] = s.Revision
			}
		}
	}
	metrics := accountMetrics(a, func(sid int64) *Stock { return e.stocks[sid] }, e.meta.Settings, e.clock().UnixMilli())
	digest := sha256.Sum256(encode(map[string]any{"stocks": versions, "sellable": availableLots, "borrowAccrued": metrics["borrowAccrued"], "financingAccrued": metrics["financingAccrued"], "availableCash": availableCash(a, e.clock().UnixMilli())}))
	return fmt.Sprintf("\"portfolio-%d-%d-%x\"", a.ID, a.Revision, digest[:12]), nil
}
