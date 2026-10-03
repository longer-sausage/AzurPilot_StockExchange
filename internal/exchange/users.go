package exchange

import (
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type AdminPlayerSummary struct {
	ID            int64  `json:"id"`
	Symbol        string `json:"symbol"`
	Username      string `json:"username"`
	IdentityCode  string `json:"identityCode"`
	Cash          int64  `json:"cash"`
	Equity        int64  `json:"equity"`
	Available     int64  `json:"available"`
	JoinedAt      int64  `json:"joinedAt"`
	Disabled      bool   `json:"disabled"`
	Stale         bool   `json:"stale"`
	InstanceID    string `json:"instanceId"`
	Positions     int    `json:"positionCount"`
	PendingOrders int    `json:"pendingOrders"`
	Quote         Quote  `json:"quote"`
}

type AdminPlayerList struct {
	Players  []AdminPlayerSummary `json:"players"`
	Total    int                  `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"pageSize"`
	Active   int                  `json:"active"`
	Disabled int                  `json:"disabled"`
}

func (e *Engine) AdminPlayers(query, status string, page, pageSize int) AdminPlayerList {
	e.mu.RLock()
	defer e.mu.RUnlock()
	query = cases.Fold().String(norm.NFKC.String(strings.TrimSpace(query)))
	out := AdminPlayerList{Players: []AdminPlayerSummary{}, Page: page, PageSize: pageSize}
	ids := []int64{}
	for id, p := range e.players {
		if p.Disabled {
			out.Disabled++
		} else {
			out.Active++
		}
		if status == "active" && p.Disabled || status == "disabled" && !p.Disabled {
			continue
		}
		instanceID := ""
		if p.Binding != nil {
			instanceID = p.Binding.InstanceID
		}
		if query != "" && !strings.Contains(cases.Fold().String(p.Username+" "+symbol(id)+" "+p.IdentityCode+" "+instanceID), query) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out.Total = len(ids)
	// 删除筛选条件或记录变动时，返回最后一个有效页面。
	out.Page = min(page, max(1, (out.Total+pageSize-1)/pageSize))
	start := (out.Page - 1) * pageSize
	now := e.clock().Unix()
	for _, id := range ids[start:min(start+pageSize, len(ids))] {
		p := e.players[id]
		a := e.account(p, now)
		v := AdminPlayerSummary{ID: id, Symbol: symbol(id), Username: p.Username, IdentityCode: p.IdentityCode, Cash: p.Cash, Equity: a.Equity, Available: a.Available, JoinedAt: p.JoinedAt, Disabled: p.Disabled, Stale: !e.quoteFresh(p, now), Positions: len(p.Positions), Quote: p.Quote}
		if p.Binding != nil {
			v.InstanceID = p.Binding.InstanceID
		}
		for _, o := range p.Orders {
			if o.Status == "pending" {
				v.PendingOrders++
			}
		}
		out.Players = append(out.Players, v)
	}
	return out
}

type AdminPlayerUpdate struct {
	Username       *string `json:"username"`
	Password       *string `json:"password"`
	CashAdjustment *int64  `json:"cashAdjustment"`
	ExpectedCash   *int64  `json:"expectedCash"`
	Disabled       *bool   `json:"disabled"`
}

func (e *Engine) UpdatePlayer(id int64, in AdminPlayerUpdate) (Account, error) {
	if in.Username == nil && in.Password == nil && in.CashAdjustment == nil && in.Disabled == nil {
		return Account{}, fail("INVALID_UPDATE", "请至少修改一项用户信息")
	}
	name, folded, passwordHash := "", "", ""
	if in.Username != nil {
		var err error
		name, folded, err = normalizeName(*in.Username)
		if err != nil {
			return Account{}, err
		}
	}
	if in.Password != nil {
		if len(*in.Password) < 10 || len(*in.Password) > 72 {
			return Account{}, fail("INVALID_PASSWORD", "密码需为 10–72 字节")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*in.Password), bcrypt.DefaultCost)
		if err != nil {
			return Account{}, err
		}
		passwordHash = string(hash)
	}
	if in.CashAdjustment != nil && (in.ExpectedCash == nil || *in.CashAdjustment < -MaxNotional || *in.CashAdjustment > MaxNotional) {
		return Account{}, fail("INVALID_CASH", "现金调整须携带原现金余额，调整金额不得超过 1 万亿模拟币")
	}
	if in.ExpectedCash != nil && in.CashAdjustment == nil {
		return Account{}, fail("INVALID_CASH", "缺少现金调整金额")
	}
	var result Account
	err := e.transaction(func() error {
		now := e.clock()
		if err := e.advance(now); err != nil {
			return err
		}
		p := e.players[id]
		if p == nil {
			return fail("NOT_FOUND", "账户不存在")
		}
		if in.Username != nil && e.names[folded] != 0 && e.names[folded] != id {
			return fail("USERNAME_TAKEN", "用户名已被注册，请换一个")
		}
		if in.CashAdjustment != nil && p.Cash != *in.ExpectedCash {
			return fail("CASH_CHANGED", "用户现金已发生变动，请重新加载详情后调整")
		}
		p = e.touch(id)
		if in.Username != nil {
			p.Username, p.Folded = name, folded
		}
		if in.Password != nil {
			p.PasswordHash = passwordHash
			p.SessionVersion++
		}
		if in.CashAdjustment != nil && *in.CashAdjustment != 0 {
			e.accrue(p, now.Unix())
			cash := p.Cash + *in.CashAdjustment
			if cash < 0 || cash > MaxNotional {
				return fail("INVALID_CASH", "调整后的现金须为 0 至 1 万亿模拟币")
			}
			p.Cash = cash
			a := e.account(p, now.Unix())
			if *in.CashAdjustment < 0 && (p.Cash < a.Unsettled+a.Frozen+a.BorrowAccrued+a.FinancingAccrued || a.Equity < a.MaintenanceMargin) {
				return fail("CASH_IN_USE", "扣减金额超过可用资金或会导致保证金不足，请先处理持仓与挂单")
			}
		}
		if in.Disabled != nil && p.Disabled != *in.Disabled {
			e.disablePlayer(p, *in.Disabled, now)
		}
		result = e.account(clonePlayer(p), now.Unix())
		return nil
	})
	return result, err
}

func (e *Engine) disablePlayer(p *Player, disabled bool, now time.Time) {
	p.Disabled = disabled
	if !disabled {
		return
	}
	p.SessionVersion++
	e.accrue(p, now.Unix())
	e.forcedClose(p, now, "管理员停用账户")
	for watcher := range e.watchers[p.ID] {
		v := e.touch(watcher)
		for _, o := range v.Orders {
			if o.StockID == p.ID && o.Status == "pending" {
				o.Status, o.Reserved, o.Reason = "cancelled", 0, "股票停牌"
			}
		}
	}
}
