package exchange

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Engine struct {
	mu                     sync.RWMutex
	store                  Store
	clock                  Clock
	meta                   Meta
	accounts               map[int64]*Account
	stocks                 map[int64]*Stock
	orders                 map[int64]*Order
	retiredRegistrationIDs []string
	resets                 map[string]*ResetReceipt
	previews               map[string]*Preview
	changes                chan struct{}
	cachedMarket           []byte
	cachedVersion          uint64
	nextTick               int64
	fresh                  bool
	chartMu                sync.Mutex
	chartCache             map[string]chartCacheEntry
	chartAccess            uint64
	rulesBytes             []byte
	rulesTag               string
	byClient               map[string]int64
	byPreview              map[string]int64
	pending                map[int64]bool
	accountOrders          map[int64][]int64
	byOfficial             map[string]int64
	byUsername             map[string]int64
	byInstance             map[string]int64
	shortOwners            map[int64]map[int64]bool
	longOwners             map[int64]map[int64]bool
	archives               map[string]*SeasonArchive
	trades                 map[int64]*Trade
	stockTrades            map[int64][]int64
}
type mutation struct {
	e                    *Engine
	meta                 Meta
	accounts             map[int64]*Account
	stocks               map[int64]*Stock
	orders               map[int64]*Order
	resets               map[string]*ResetReceipt
	removedRegistrations []string
	history              []HistoryWrite
	changed              bool
	rulesChanged         bool
	marketChanged        bool
	archives             map[string]*SeasonArchive
	trades               []*Trade
}
type ResetReceipt struct {
	PasswordVersion int64  `json:"passwordVersion,omitempty"`
	Username        string `json:"username"`
	OfficialUserID  string `json:"officialUserId"`
	Ciphertext      string `json:"ciphertext"`
	ExpiresAt       int64  `json:"expiresAt"`
}

func Open(database string, clock Clock) (*Engine, error) {
	store, err := OpenPostgres(database)
	if err != nil {
		return nil, err
	}
	e, err := NewEngine(store, clock)
	if err != nil {
		store.Close()
	}
	return e, err
}
func NewEngine(store Store, clock Clock) (*Engine, error) {
	if clock == nil {
		clock = time.Now
	}
	e := &Engine{store: store, clock: clock, meta: Meta{Version: 1, NextAccount: 1, NextStock: 1, NextOrder: 1, Settings: DefaultSettings()}, accounts: map[int64]*Account{}, stocks: map[int64]*Stock{}, orders: map[int64]*Order{}, resets: map[string]*ResetReceipt{}, previews: map[string]*Preview{}, changes: make(chan struct{})}
	entities, err := store.Load()
	if err != nil {
		return nil, err
	}
	e.fresh = true
	e.chartCache = map[string]chartCacheEntry{}
	e.archives = map[string]*SeasonArchive{}
	e.trades = map[int64]*Trade{}
	e.stockTrades = map[int64][]int64{}
	for _, v := range entities {
		switch v.Kind {
		case "trade":
			var x Trade
			err = json.Unmarshal(v.Data, &x)
			e.trades[x.ID] = &x
			e.indexTrade(&x)
		case "meta":
			e.fresh = false
			err = json.Unmarshal(v.Data, &e.meta)
		case "account":
			var x Account
			err = json.Unmarshal(v.Data, &x)
			if x.Positions == nil {
				x.Positions = map[int64]*Position{}
			}
			e.accounts[x.ID] = &x
		case "stock":
			var x Stock
			err = json.Unmarshal(v.Data, &x)
			e.stocks[x.ID] = &x
		case "order":
			var x Order
			err = json.Unmarshal(v.Data, &x)
			e.orders[x.ID] = &x
		case "registration":
			e.retiredRegistrationIDs = append(e.retiredRegistrationIDs, v.ID)
		case "reset":
			var x ResetReceipt
			err = json.Unmarshal(v.Data, &x)
			e.resets[v.ID] = &x
		case "season":
			var x SeasonArchive
			err = json.Unmarshal(v.Data, &x)
			e.archives[v.ID] = &x
		}
		if err != nil {
			return nil, err
		}
	}
	if err = ValidateSettings(e.meta.Settings); err != nil {
		return nil, err
	}
	e.initializeIndexes()
	if e.meta.NextTrade == 0 {
		e.meta.NextTrade = 1
	}
	for id := range e.trades {
		if id >= e.meta.NextTrade {
			e.meta.NextTrade = id + 1
		}
	}
	if e.meta.Season.ID == "" {
		window := SeasonFor(clock(), e.meta.Settings.StartDay, e.meta.Settings.EndDaysFromLast)
		e.meta.Season = Season{ID: window.ID, StartsAt: window.StartsAt, EndsAt: window.EndsAt}
	}
	if err = e.migrateFeatures(); err != nil {
		return nil, err
	}
	return e, nil
}
func (e *Engine) Close() error { return e.store.Close() }

// InitializeDefaults 仅在空的 v2 数据库上读取默认配置，重启不会覆盖管理面板配置。
func (e *Engine) InitializeDefaults(path string) error {
	e.mu.RLock()
	fresh := e.fresh
	e.mu.RUnlock()
	if !fresh {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	settings := DefaultSettings()
	if err = json.Unmarshal(raw, &settings); err != nil {
		return err
	}
	if settings.MarketPresets, err = LoadMarketPresets(filepath.Join(filepath.Dir(path), "presets")); err != nil {
		return err
	}
	var defaultSelection struct {
		ActiveMarketID string          `json:"activeMarketId"`
		ActiveMarket   json.RawMessage `json:"activeMarket"`
	}
	if err = json.Unmarshal(raw, &defaultSelection); err != nil {
		return err
	}
	if len(defaultSelection.ActiveMarket) == 0 {
		selected := defaultSelection.ActiveMarketID
		if selected == "" {
			selected = "meow"
		}
		found := false
		for _, r := range settings.MarketPresets {
			if r.ID == selected {
				settings.ActiveMarket = r
				found = true
				break
			}
		}
		if !found {
			return fail("INVALID_RULES", "默认市场预设文件缺失")
		}
	}
	if err = e.SetSettings(settings); err != nil {
		return err
	}
	e.mu.Lock()
	e.fresh = false
	e.mu.Unlock()
	return nil
}
func (e *Engine) transaction(fn func(*mutation) error) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	m := &mutation{e: e, meta: e.meta, accounts: map[int64]*Account{}, stocks: map[int64]*Stock{}, orders: map[int64]*Order{}, resets: map[string]*ResetReceipt{}, archives: map[string]*SeasonArchive{}}
	if err := fn(m); err != nil {
		return err
	}
	if !m.changed {
		return nil
	}
	m.meta.Version++
	if len(m.stocks) > 0 || m.marketChanged {
		m.meta.MarketVersion++
	}
	entities := []Entity{{Kind: "meta", ID: "1", Data: encode(m.meta)}}
	for id, x := range m.accounts {
		x.Revision++
		if x.Realized > LedgerLimit || x.Realized < -LedgerLimit {
			return fail("NUMERICAL_LIMIT", "累计损益超出整数安全范围")
		}
		if x.Cash < 0 || x.Reserved < 0 || x.Cash > LedgerLimit || x.Reserved > x.Cash {
			return fail("NUMERICAL_LIMIT", "账户余额或冻结超出安全范围")
		}
		entities = append(entities, Entity{Kind: "account", ID: idstr(id), Data: encode(x)})
	}
	for id, x := range m.stocks {
		x.Revision++
		entities = append(entities, Entity{Kind: "stock", ID: idstr(id), Data: encode(x)})
	}
	for id, x := range m.orders {
		entities = append(entities, Entity{Kind: "order", ID: idstr(id), Data: encode(x)})
	}
	for _, id := range m.removedRegistrations {
		entities = append(entities, Entity{Kind: "registration", ID: id, Delete: true})
	}
	for id, x := range m.resets {
		entities = append(entities, Entity{Kind: "reset", ID: id, Data: encode(x)})
	}
	for id, x := range m.archives {
		entities = append(entities, Entity{Kind: "season", ID: id, Data: encode(x)})
	}
	for _, x := range m.trades {
		entities = append(entities, Entity{Kind: "trade", ID: idstr(x.ID), Data: encode(x)})
	}
	if err := e.store.Commit(entities, m.history); err != nil {
		return err
	}
	e.meta = m.meta
	for _, x := range m.trades {
		e.trades[x.ID] = x
		e.indexTrade(x)
	}
	if m.rulesChanged {
		e.rulesBytes = nil
		e.nextTick = 0
	}
	for id, x := range m.accounts {
		e.indexAccount(x)
		e.accounts[id] = x
	}
	for id, x := range m.stocks {
		e.stocks[id] = x
		e.byInstance[x.InstanceID] = id
	}
	for id, x := range m.orders {
		e.indexOrder(x, e.orders[id] == nil)
		e.orders[id] = x
	}
	if len(m.removedRegistrations) > 0 {
		e.retiredRegistrationIDs = nil
	}
	for id, x := range m.resets {
		e.resets[id] = x
	}
	for id, x := range m.archives {
		e.archives[id] = x
	}
	if len(m.stocks) > 0 || m.marketChanged {
		e.cachedMarket = nil
	}
	close(e.changes)
	e.changes = make(chan struct{})
	return nil
}
func (m *mutation) account(id int64) *Account {
	if p := m.accounts[id]; p != nil {
		return p
	}
	old := m.e.accounts[id]
	if old == nil {
		return nil
	}
	p := *old
	p.Watchlist = append([]int64{}, old.Watchlist...)
	p.Settlements = append([]Settlement{}, old.Settlements...)
	p.Positions = make(map[int64]*Position, len(old.Positions))
	for id, pos := range old.Positions {
		x := *pos
		x.Lots = append([]Lot{}, pos.Lots...)
		p.Positions[id] = &x
	}
	m.accounts[id] = &p
	m.changed = true
	return &p
}
func (m *mutation) stock(id int64) *Stock {
	if x := m.stocks[id]; x != nil {
		return x
	}
	old := m.e.stocks[id]
	if old == nil {
		return nil
	}
	x := *old
	m.stocks[id] = &x
	m.changed = true
	return &x
}
func (m *mutation) order(id int64) *Order {
	if x := m.orders[id]; x != nil {
		return x
	}
	old := m.e.orders[id]
	if old == nil {
		return nil
	}
	x := *old
	m.orders[id] = &x
	m.changed = true
	return &x
}
func (m *mutation) readStock(id int64) *Stock {
	if x := m.stocks[id]; x != nil {
		return x
	}
	return m.e.stocks[id]
}
func (m *mutation) readAccount(id int64) *Account {
	if x := m.accounts[id]; x != nil {
		return x
	}
	return m.e.accounts[id]
}
func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func strongPassword() string { return "Aa7!" + randomToken(24) }
func publicStock(s *Stock) map[string]any {
	return map[string]any{"id": s.ID, "historyCheckedAt": s.HistoryCheckedAt, "open": s.Open, "openAt": s.OpenAt, "openDay": s.OpenDay, "disabled": s.Disabled, "deletedAt": s.DeletedAt, "uploadedAt": s.UploadedAt, "tradeVersion": s.TradeVersion, "symbol": s.Symbol, "name": s.Name, "price": s.Price, "previous": s.Previous, "observedAt": s.ObservedAt, "historyVersion": s.HistoryVersion, "bound": s.AccountID != 0, "issuedShares": s.IssuedShares, "hourlyVolume": s.HourlyVolume, "borrowShares": s.BorrowShares, "borrowHourlyVolume": s.BorrowHourlyVolume, "normalUsed": s.NormalUsed, "borrowUsed": s.BorrowUsed, "outstanding": s.Outstanding, "borrowed": s.Borrowed, "delisted": s.Delisted}
}
func (e *Engine) Market() ([]byte, uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cachedMarket != nil {
		return e.cachedMarket, e.meta.MarketVersion
	}
	stocks := make([]map[string]any, 0, len(e.stocks))
	ids := make([]int64, 0, len(e.stocks))
	for id := range e.stocks {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if e.stocks[id].DeletedAt != 0 {
			continue
		}
		stocks = append(stocks, publicStock(e.stocks[id]))
	}
	m := &mutation{e: e, meta: e.meta}
	open, reason := m.marketOpen(e.clock().UnixMilli())
	e.cachedMarket = encode(map[string]any{"version": e.meta.MarketVersion, "stocks": stocks, "serverTime": e.clock().UnixMilli(), "open": open, "reason": reason, "season": e.meta.Season})
	return e.cachedMarket, e.meta.MarketVersion
}
func (e *Engine) Settings() Settings {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s := e.meta.Settings
	_ = json.Unmarshal(encode(s), &s)
	return s
}
func (e *Engine) SetSettings(s Settings) error {
	var copySettings Settings
	_ = json.Unmarshal(encode(s), &copySettings)
	s = copySettings
	if err := ValidateSettings(s); err != nil {
		return err
	}
	return e.transaction(func(m *mutation) error {
		oldOpen, oldReason := m.marketOpen(e.clock().UnixMilli())
		oldWindow := m.meta.Season
		oldThreshold := m.meta.Settings.DelistThreshold
		oldProfile := m.meta.Settings.ActiveMarket.ID
		for id, a := range e.accounts {
			for sid, p := range a.Positions {
				if p.ShortQuantity > 0 || p.Loan > 0 {
					if err := m.accrue(m.account(id), sid, e.clock().UnixMilli()); err != nil {
						return err
					}
				}
			}
		}
		m.meta.Settings = s
		if len(e.accounts) == 0 && len(e.stocks) == 0 {
			window := SeasonFor(e.clock(), s.StartDay, s.EndDaysFromLast)
			m.meta.Season = Season{ID: window.ID, StartsAt: window.StartsAt, EndsAt: window.EndsAt}
		}
		if oldProfile != s.ActiveMarket.ID {
			for oid := range e.pending {
				o := m.order(oid)
				if o.Status == "pending" {
					m.cancel(m.account(o.AccountID), o, e.clock().UnixMilli(), "市场预设切换，请重新预览委托")
				}
			}
		}

		m.rulesChanged = true
		newOpen, newReason := m.marketOpen(e.clock().UnixMilli())
		m.marketChanged = oldOpen != newOpen || oldReason != newReason || oldWindow != m.meta.Season
		m.changed = true
		if oldThreshold != s.DelistThreshold {
			for id := range e.stocks {
				if err := m.delistStock(m.stock(id), e.clock().UnixMilli()); err != nil {
					return err
				}
			}
		}
		for sid, owners := range e.shortOwners {
			if len(owners) > 0 {
				if err := m.liquidateStock(m.stock(sid), e.clock().UnixMilli()); err != nil {
					return err
				}
			}
		}
		for sid, owners := range e.longOwners {
			for accountID := range owners {
				if err := m.riskLong(m.account(accountID), m.stock(sid), e.clock().UnixMilli()); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
func equity(a *Account, stock func(int64) *Stock) int64 {
	v := a.Cash
	for id, p := range a.Positions {
		if s := stock(id); s != nil {
			delta := p.Quantity*s.Price + p.ShortProceeds + p.Collateral - p.ShortQuantity*s.Price - p.Loan
			if delta > 0 && v > LedgerLimit-delta {
				v = LedgerLimit
			} else if delta < 0 && v < -LedgerLimit-delta {
				v = -LedgerLimit
			} else {
				v += delta
			}
		}
	}
	return v
}
func (e *Engine) RulesData() ([]byte, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.rulesBytes == nil {
		e.rulesBytes = encode(e.meta.Settings)
		digest := sha256.Sum256(e.rulesBytes)
		e.rulesTag = fmt.Sprintf("\"rules-%x\"", digest[:12])
	}
	return e.rulesBytes, e.rulesTag
}
func accountPublic(a *Account, stock func(int64) *Stock) map[string]any {
	return map[string]any{"id": a.ID, "metricsSince": a.MetricsSince, "legacyMetrics": a.LegacyMetrics, "identityCode": a.IdentityCode, "watchlist": a.Watchlist, "initialCash": a.InitialCash, "returnPPM": returnPPM(equity(a, stock), a.InitialCash), "realized": a.Realized, "fees": a.Fees, "username": a.Username, "cash": a.Cash, "reserved": a.Reserved, "equity": equity(a, stock), "boundStockId": a.BoundStockID, "forcePasswordChange": a.ForcePasswordChange, "disabled": a.Disabled, "createdAt": a.CreatedAt, "settlements": a.Settlements}
}
func (e *Engine) Portfolio(id int64) (map[string]any, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	a := e.accounts[id]
	if a == nil {
		return nil, fail("UNAUTHORIZED", "账户不可用")
	}
	lookup := func(id int64) *Stock { return e.stocks[id] }
	positions := []map[string]any{}
	for sid, p := range a.Positions {
		if p.Quantity == 0 && p.ShortQuantity == 0 {
			continue
		}
		x := map[string]any{"stockId": sid, "quantity": p.Quantity, "cost": p.Cost, "shortQuantity": p.ShortQuantity, "shortCost": p.ShortCost, "shortProceeds": p.ShortProceeds, "collateral": p.Collateral, "loan": p.Loan, "lots": p.Lots, "sellable": sellable(p, e.clock().UnixMilli())}
		if s := e.stocks[sid]; s != nil {
			x["stock"] = publicStock(s)
		}
		positions = append(positions, x)
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i]["stockId"].(int64) < positions[j]["stockId"].(int64) })
	orders := []map[string]any{}
	for _, oid := range e.accountOrders[id] {
		o := e.orders[oid]
		item := publicOrder(o)
		if stock := e.stocks[o.StockID]; stock != nil {
			item["stockName"] = stock.Name
		}
		orders = append(orders, item)
	}
	sort.Slice(orders, func(i, j int) bool { return orders[i]["id"].(int64) > orders[j]["id"].(int64) })
	account := accountPublic(a, lookup)
	settlements := []Settlement{}
	for _, x := range a.Settlements {
		if x.AvailableAt > e.clock().UnixMilli() {
			settlements = append(settlements, x)
		}
	}
	account["settlements"] = settlements
	account["availableCash"] = availableCash(a, e.clock().UnixMilli())
	account["metrics"] = accountMetrics(a, lookup, e.meta.Settings, e.clock().UnixMilli())
	trades := []*Trade{}
	for _, t := range e.trades {
		if t.AccountID == id {
			trades = append(trades, t)
		}
	}
	sort.Slice(trades, func(i, j int) bool { return trades[i].ID > trades[j].ID })
	return map[string]any{"account": account, "positions": positions, "orders": orders, "trades": trades}, nil
}
func (e *Engine) resolve(query string, accountID int64) (*Stock, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		if a := e.accounts[accountID]; a != nil && a.BoundStockID != 0 {
			return e.stocks[a.BoundStockID], nil
		}
		return nil, fail("NO_BOUND_STOCK", "交易账户尚未绑定股票")
	}
	if strings.HasPrefix(query, "official:") {
		official := strings.TrimPrefix(query, "official:")
		for _, a := range e.accounts {
			if a.OfficialUserID == official && a.BoundStockID != 0 {
				return e.stocks[a.BoundStockID], nil
			}
		}
		return nil, fail("NOT_FOUND", "该官方机器人用户未绑定股票")
	}
	for _, s := range e.stocks {
		if s.DeletedAt == 0 && (s.Symbol == query || normalized(s.Name) == normalized(query)) {
			return s, nil
		}
	}
	if id, err := strconv.ParseInt(query, 10, 64); err == nil {
		if s := e.stocks[id]; s != nil && s.DeletedAt == 0 {
			return s, nil
		}
	}
	for _, a := range e.accounts {
		if normalized(a.Username) == normalized(query) && a.BoundStockID != 0 {
			return e.stocks[a.BoundStockID], nil
		}
	}
	return nil, fail("NOT_FOUND", "找不到指定股票")
}
func (e *Engine) Stock(query string, id int64) (map[string]any, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, err := e.resolve(query, id)
	if err != nil {
		return nil, err
	}
	return publicStock(s), nil
}
func (e *Engine) Bind(id int64, code string) (map[string]any, error) {
	var out map[string]any
	err := e.transaction(func(m *mutation) error {
		a := m.readAccount(id)
		if a == nil || a.Disabled {
			return fail("UNAUTHORIZED", "账户不可用")
		}
		var target *Stock
		for _, s := range e.stocks {
			if s.DeletedAt == 0 && s.BindingCode != "" && s.BindingCode == strings.TrimSpace(code) {
				target = s
				break
			}
		}
		if target == nil {
			return fail("INVALID_BINDING_CODE", "股票绑定码无效")
		}
		if a.BoundStockID != 0 && a.BoundStockID != target.ID || target.AccountID != 0 && target.AccountID != id {
			return fail("ALREADY_BOUND", "交易账户和股票只能一对一绑定；股票被管理员删除后才可重新绑定")
		}
		if target.AccountID == id {
			out = publicStock(target)
			return nil
		}
		if p := a.Positions[target.ID]; p != nil && (p.Quantity > 0 || p.ShortQuantity > 0) {
			return fail("BINDING_POSITION_CONFLICT", "请先清空该股票的持仓和借券，再绑定发行账户")
		}
		for _, o := range e.orders {
			if o.AccountID == id && o.StockID == target.ID && o.Status == "pending" {
				return fail("BINDING_POSITION_CONFLICT", "请先取消该股票的挂单，再绑定发行账户")
			}
		}
		a = m.account(id)
		s := m.stock(target.ID)
		a.BoundStockID = s.ID
		s.AccountID = id
		out = publicStock(s)
		return nil
	})
	return out, err
}
func (e *Engine) AdminAccounts() []map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := []map[string]any{}
	for _, a := range e.accounts {
		item := accountPublic(a, func(id int64) *Stock { return e.stocks[id] })
		positions, pending := 0, 0
		for _, p := range a.Positions {
			if p.Quantity > 0 || p.ShortQuantity > 0 {
				positions++
			}
		}
		for _, oid := range e.accountOrders[a.ID] {
			if e.orders[oid].Status == "pending" {
				pending++
			}
		}
		item["positionCount"], item["pendingCount"] = positions, pending
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["id"].(int64) < out[j]["id"].(int64) })
	return out
}
func (e *Engine) AdminStocks() []*Stock {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := []*Stock{}
	for _, s := range e.stocks {
		x := *s
		out = append(out, &x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (e *Engine) UpdateAccount(id int64, cash *int64, disabled *bool) error {
	return e.transaction(func(m *mutation) error {
		a := m.account(id)
		if a == nil {
			return fail("NOT_FOUND", "账户不存在")
		}
		if cash != nil {
			if *cash < a.Reserved || *cash > LedgerLimit/4 {
				return fail("INVALID_CASH", "现金不能低于冻结资金或超出安全范围")
			}
			a.Cash = *cash
		}
		if disabled != nil {
			a.Disabled = *disabled
			a.PasswordVersion++
		}
		return nil
	})
}
func (e *Engine) UpdateStock(id int64, in Stock) error {
	return e.transaction(func(m *mutation) error {
		s := m.stock(id)
		if s == nil {
			return fail("NOT_FOUND", "股票不存在")
		}
		for _, n := range []int64{in.IssuedShares, in.HourlyVolume, in.BorrowShares, in.BorrowHourlyVolume} {
			if n < 1 || n > LedgerLimit/MaxPrice {
				return fail("INVALID_RULES", "股数额度超出安全范围")
			}
		}
		if in.IssuedShares < s.Outstanding || in.BorrowShares < s.Borrowed {
			return fail("STOCK_IN_USE", "发行量或借券库存不能低于已持有数量")
		}
		s.IssuedShares = in.IssuedShares
		s.HourlyVolume = in.HourlyVolume
		s.BorrowShares = in.BorrowShares
		s.BorrowHourlyVolume = in.BorrowHourlyVolume
		return nil
	})
}
func (e *Engine) Tick() error {
	now := e.clock().UnixMilli()
	e.mu.RLock()
	due := e.nextTick
	e.mu.RUnlock()
	if due != 0 && now < due {
		return nil
	}
	err := e.transaction(func(m *mutation) error { return m.auction(now) })
	if err == nil {
		e.mu.Lock()
		e.nextTick = nextMarketBoundary(now, e.meta.Settings)
		for id := range e.pending {
			o := e.orders[id]
			if o.ExpiresAt > now && o.ExpiresAt < e.nextTick {
				e.nextTick = o.ExpiresAt
			}
		}
		for id, p := range e.previews {
			if p.ExpiresAt < now {
				delete(e.previews, id)
			}
		}
		e.mu.Unlock()
	}
	return err
}
func (e *Engine) Changes() (uint64, <-chan struct{}) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.meta.Version, e.changes
}
func (e *Engine) String() string { return fmt.Sprintf("exchange-v2") }
