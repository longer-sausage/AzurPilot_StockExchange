package exchange

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type InstanceRequest struct {
	Months     []Month `json:"months,omitempty"`
	InstanceID string  `json:"instanceId"`
	PublicKey  string  `json:"publicKey"`
	IssuedAt   int64   `json:"issuedAt"`
	Signature  string  `json:"signature"`
	Username   string  `json:"username,omitempty"`
	Points     []Point `json:"points,omitempty"`
	Backfill   bool    `json:"backfill,omitempty"`
}

var instancePattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

func instanceCanonical(action string, r InstanceRequest) string {
	points := r.Points
	if points == nil {
		points = []Point{}
	}
	username := ""
	if action == "register" {
		username = r.Username
	}
	text := fmt.Sprintf("mmex-v2\n%s\n%s\n%s\n%d\n%s\n%s\n", action, r.InstanceID, r.PublicKey, r.IssuedAt, username, encode(points))
	if action == "reconcile" {
		months := r.Months
		if months == nil {
			months = []Month{}
		}
		text += string(encode(months)) + "\n"
	}
	return text
}
func (e *Engine) verifyInstance(action string, r InstanceRequest) error {
	if !instancePattern.MatchString(r.InstanceID) || r.IssuedAt < e.clock().Unix()-300 || r.IssuedAt > e.clock().Unix()+300 {
		return fail("INVALID_INSTANCE", "实例身份或签名时间无效")
	}
	pub, err := base64.StdEncoding.DecodeString(r.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fail("INVALID_INSTANCE", "实例公钥无效")
	}
	sig, err := base64.StdEncoding.DecodeString(r.Signature)
	if err != nil || !ed25519.Verify(pub, []byte(instanceCanonical(action, r)), sig) {
		return fail("INVALID_SIGNATURE", "实例签名无效")
	}
	return nil
}
func (e *Engine) instanceStock(r InstanceRequest) (*Stock, error) {
	if s := e.stocks[e.byInstance[r.InstanceID]]; s != nil {
		if s.PublicKey != r.PublicKey {
			return nil, fail("INSTANCE_KEY_MISMATCH", "实例已由其他密钥注册")
		}
		if s.DeletedAt != 0 {
			return nil, fail("STOCK_DELETED", "股票已被管理员删除，当前实例必须停止上报；原身份不能重新注册")
		}
		return s, nil
	}
	return nil, fail("STOCK_NOT_REGISTERED", "当前实例尚未注册股票")
}
func instanceStatus(s *Stock) map[string]any {
	return map[string]any{"stock": publicStock(s), "bindingCode": s.BindingCode, "exchangeUrl": "https://stock.azurpilot.com", "cursor": s.Cursor, "capabilities": []string{"reconcile"}}
}
func (e *Engine) InstanceRegister(r InstanceRequest) (map[string]any, error) {
	if err := e.verifyInstance("register", r); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(r.Username)
	if len([]rune(name)) < 2 || len([]rune(name)) > 32 || strings.ContainsAny(name, "\r\n\x00") {
		return nil, fail("INVALID_USERNAME", "股票名称须为 2–32 个字符")
	}
	var out map[string]any
	err := e.transaction(func(m *mutation) error {
		if old, err := e.instanceStock(r); err == nil {
			out = instanceStatus(old)
			return nil
		} else if publicError(err).Code != "STOCK_NOT_REGISTERED" {
			return err
		}
		for _, s := range e.stocks {
			if s.DeletedAt == 0 && canonicalName(s.Name) == canonicalName(name) {
				return fail("NAME_TAKEN", "股票名称已被使用")
			}
			if s.PublicKey == r.PublicKey {
				return fail("KEY_ALREADY_REGISTERED", "该实例密钥已注册其他股票")
			}
		}
		id := m.meta.NextStock
		m.meta.NextStock++
		s := &Stock{ID: id, Symbol: fmt.Sprintf("MM%06d", id), Name: name, InstanceID: r.InstanceID, PublicKey: r.PublicKey, BindingCode: randomToken(24), IssuedShares: m.meta.Settings.IssuedShares, HourlyVolume: m.meta.Settings.HourlyVolume, BorrowShares: m.meta.Settings.BorrowShares, BorrowHourlyVolume: m.meta.Settings.BorrowHourlyVolume, Hour: e.clock().Unix() / 3600}
		m.stocks[id] = s
		m.changed = true
		out = instanceStatus(s)
		return nil
	})
	return out, err
}
func (e *Engine) InstanceStatus(r InstanceRequest) (map[string]any, error) {
	if err := e.verifyInstance("status", r); err != nil {
		return nil, err
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, err := e.instanceStock(r)
	if err != nil {
		return nil, err
	}
	return instanceStatus(s), nil
}
func (e *Engine) InstanceManifest(r InstanceRequest) ([]Month, error) {
	if err := e.verifyInstance("manifest", r); err != nil {
		return nil, err
	}
	e.mu.RLock()
	s, err := e.instanceStock(r)
	var id int64
	if s != nil {
		id = s.ID
	}
	e.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	return e.store.Manifest(id)
}
func (e *Engine) InstanceReport(r InstanceRequest) (map[string]any, error) {
	if err := e.verifyInstance("report", r); err != nil {
		return nil, err
	}
	if len(r.Points) == 0 {
		return nil, fail("INVALID_POINTS", "上报数据不能为空")
	}
	pointsMap := make(map[int64]int64, len(r.Points))
	for _, p := range r.Points {
		if p.T < 0 || p.T > e.clock().UnixMilli()+300000 || p.P < 0 || p.P > MaxPrice/100 {
			return nil, fail("INVALID_POINTS", "行动力或毫秒时间超出范围")
		}
		pointsMap[p.T] = p.P
	}
	points := make([]Point, 0, len(pointsMap))
	for t, p := range pointsMap {
		points = append(points, Point{t, p})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].T < points[j].T })
	var out map[string]any
	err := e.transaction(func(m *mutation) error {
		old, err := e.instanceStock(r)
		if err != nil {
			return err
		}
		s := m.stock(old.ID)
		s.UploadedAt = e.clock().UnixMilli()
		if err := m.updateOpening(s, points); err != nil {
			return err
		}
		if err := m.advanceSeason(e.clock().UnixMilli()); err != nil {
			return err
		}
		last := points[len(points)-1]
		if last.T >= s.ObservedAt && last.P*100 != s.Price {
			for id := range e.shortOwners[s.ID] {
				if err := m.accrue(m.account(id), s.ID, e.clock().UnixMilli()); err != nil {
					return err
				}
			}
		}
		if last.T >= s.ObservedAt {
			if last.T > s.ObservedAt {
				s.Previous = s.Price
			}
			s.Price = last.P * 100
			s.ObservedAt = last.T
		}
		if last.T > s.Cursor {
			s.Cursor = last.T
		}
		m.history = append(m.history, HistoryWrite{s.ID, points})
		s.HistoryVersion++
		if last.T >= old.ObservedAt {
			if err = m.liquidateStock(s, e.clock().UnixMilli()); err != nil {
				return err
			}
			for accountID := range e.longOwners[s.ID] {
				if err = m.riskLong(m.account(accountID), s, e.clock().UnixMilli()); err != nil {
					return err
				}
			}
			if !r.Backfill {
				if err = m.delistStock(s, e.clock().UnixMilli()); err != nil {
					return err
				}
			}
		}
		out = map[string]any{"cursor": s.Cursor, "accepted": len(points)}
		return nil
	})
	return out, err
}
func (e *Engine) Chart(id, from, to, interval int64) ([]Candle, int64, error) {
	if from < 0 || to < from || to > 8640000000000000 || interval < 1 || interval > 525600 {
		return nil, 0, fail("INVALID_RANGE", "图表时间范围无效")
	}
	step := interval * 60000
	e.mu.RLock()
	exists := e.stocks[id] != nil && e.stocks[id].DeletedAt == 0
	e.mu.RUnlock()
	if !exists {
		return nil, 0, fail("NOT_FOUND", "股票不存在")
	}
	points, err := e.store.Chart(id, from, to, step)
	return points, step / 60000, err
}

func (e *Engine) InstanceReconcile(r InstanceRequest) (map[string]any, error) {
	if err := e.verifyInstance("reconcile", r); err != nil {
		return nil, err
	}
	var out map[string]any
	err := e.transaction(func(m *mutation) error {
		s, err := e.instanceStock(r)
		if err != nil {
			return err
		}
		months, err := e.store.Manifest(s.ID)
		if err != nil {
			return err
		}
		if len(months) != len(r.Months) {
			return fail("HISTORY_MISMATCH", "月份摘要不匹配，请先补传缺口")
		}
		expected := map[string]Month{}
		for _, month := range r.Months {
			if _, exists := expected[month.Month]; exists {
				return fail("HISTORY_MISMATCH", "月份摘要不能重复")
			}
			expected[month.Month] = month
		}
		for _, month := range months {
			if expected[month.Month] != month {
				return fail("HISTORY_MISMATCH", "月份记录数或摘要不匹配，请先补传")
			}
		}
		if s.HistoryCheckedAt == 0 || s.CheckedHistoryVersion != s.HistoryVersion {
			s = m.stock(s.ID)
			s.HistoryCheckedAt = e.clock().UnixMilli()
			s.CheckedHistoryVersion = s.HistoryVersion
		}
		out = map[string]any{"checkedAt": s.HistoryCheckedAt, "historyVersion": s.CheckedHistoryVersion}
		return nil
	})
	return out, err
}
