package exchange

import (
	"net/http"
	"strconv"
	"strings"
)

type AccountEdit struct {
	Username       *string `json:"username"`
	Cash           *int64  `json:"cash"`
	CashAdjustment *int64  `json:"cashAdjustment"`
	ExpectedCash   *int64  `json:"expectedCash"`
	Disabled       *bool   `json:"disabled"`
	ConfirmDisable bool    `json:"confirmDisable"`
}

func (e *Engine) EditAccount(id int64, in AccountEdit) error {
	return e.transaction(func(m *mutation) error {
		a := m.account(id)
		if a == nil {
			return fail("NOT_FOUND", "账户不存在")
		}
		if in.Username != nil {
			name := strings.TrimSpace(*in.Username)
			if !validName(name) {
				return fail("INVALID_USERNAME", "用户名须为 2–32 个字符")
			}
			if e.nameTaken(name, id) {
				return fail("NAME_TAKEN", "用户名已被使用")
			}
			a.Username = name
		}
		if in.Cash != nil && in.CashAdjustment != nil {
			return fail("INVALID_CASH", "绝对余额与增量不能同时设置")
		}
		if in.Cash != nil || in.CashAdjustment != nil {
			if in.ExpectedCash == nil || *in.ExpectedCash != a.Cash {
				return fail("CASH_CHANGED", "余额已经变化，请刷新账户后重新调整")
			}
			next := a.Cash
			if in.Cash != nil {
				next = *in.Cash
			} else {
				delta := *in.CashAdjustment
				if delta > LedgerLimit || delta < -LedgerLimit || delta > 0 && a.Cash > LedgerLimit-delta {
					return fail("INVALID_CASH", "现金调整超出安全范围")
				}
				next += delta
			}
			metrics := accountMetrics(a, m.readStock, m.meta.Settings, e.clock().UnixMilli())
			if next < a.Cash-availableCash(a, e.clock().UnixMilli()) || next < 0 || next > LedgerLimit/4 {
				return fail("INVALID_CASH", "现金不能低于挂单冻结与待交收金额或超过安全范围")
			}
			a.Cash = next
			if equity(a, m.readStock)-metrics["financingAccrued"]-metrics["borrowAccrued"] < metrics["maintenanceMargin"] {
				return fail("INVALID_CASH", "调整后资产不足以覆盖维持保证金与未落账费用")
			}
		}
		if in.Disabled != nil && *in.Disabled != a.Disabled {
			if *in.Disabled {
				if !in.ConfirmDisable {
					return fail("CONFIRMATION_REQUIRED", "停用将撤单并结清本人全部持仓，必须二次确认")
				}
				now := e.clock().UnixMilli()
				for oid := range e.pending {
					if e.orders[oid].AccountID == id {
						o := m.order(oid)
						if o.Status == "pending" {
							m.cancel(a, o, now, "管理员停用账户")
						}
					}
				}
				for sid := range a.Positions {
					if s := m.stock(sid); s != nil {
						if err := m.closePosition(a, s, now, "管理员停用账户，结清持仓"); err != nil {
							return err
						}
					}
				}
			}
			a.Disabled = *in.Disabled
			a.PasswordVersion++
		}
		return nil
	})
}
func (e *Engine) SetStockDisabled(id int64, disabled bool) error {
	return e.transaction(func(m *mutation) error {
		s := m.stock(id)
		if s == nil || s.DeletedAt != 0 {
			return fail("NOT_FOUND", "股票不存在")
		}
		s.Disabled = disabled
		if disabled {
			for oid := range e.pending {
				if e.orders[oid].StockID == id {
					o := m.order(oid)
					if o.Status == "pending" {
						m.cancel(m.account(o.AccountID), o, e.clock().UnixMilli(), "股票停牌，撤销挂单")
					}
				}
			}
		}
		return nil
	})
}
func (s *Server) stockStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Disabled bool `json:"disabled"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s.result(w, r, map[string]bool{"ok": true}, s.engine.SetStockDisabled(id, in.Disabled))
}
func (s *Server) adminReset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirmed bool   `json:"confirmed"`
		RequestID string `json:"requestId"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	if !in.Confirmed {
		s.result(w, r, nil, fail("CONFIRMATION_REQUIRED", "请先确认重置密码并撤销全部旧会话"))
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s.engine.mu.RLock()
	a := s.engine.accounts[id]
	official, username := "", ""
	if a != nil {
		official, username = a.OfficialUserID, a.Username
	}
	s.engine.mu.RUnlock()
	password, err := s.engine.ResetPassword(official, in.RequestID, s.config.SessionSecret)
	s.result(w, r, map[string]any{"username": username, "password": password, "forcePasswordChange": true}, err)
}
