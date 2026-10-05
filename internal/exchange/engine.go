package exchange

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

func normalizeName(name string) (string, string, error) {
	name = norm.NFKC.String(strings.TrimSpace(name))
	n := len([]rune(name))
	if n < 2 || n > 20 {
		return "", "", fail("INVALID_USERNAME", "用户名需为 2–20 位汉字、字母、数字或下划线")
	}
	for _, c := range name {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' {
			return "", "", fail("INVALID_USERNAME", "用户名只能包含汉字、字母、数字和下划线")
		}
	}
	return name, cases.Fold().String(name), nil
}
func secretToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
func hashToken(token string) string {
	v := sha256.Sum256([]byte(token))
	return hex.EncodeToString(v[:])
}
func validQuote(ap, observed int64, now time.Time) error {
	if ap < 0 || ap > 1_000_000 {
		return fail("INVALID_QUOTE", "总行动力须为 0–1,000,000 的整数")
	}
	if observed > now.Unix()+60 || observed < now.Unix()-86400 {
		return fail("INVALID_QUOTE", "仅接受最近 24 小时内的行动力记录，时间不能超前")
	}
	return nil
}
func (e *Engine) Register(username, password string, ap, observed int64, bindings ...*InstanceBinding) (*Player, string, error) {
	name, folded, err := normalizeName(username)
	if err != nil {
		return nil, "", err
	}
	if len(password) < 10 || len(password) > 72 {
		return nil, "", fail("INVALID_PASSWORD", "密码需为 10–72 字节")
	}
	if err = validQuote(ap, observed, e.clock()); err != nil {
		return nil, "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}
	token, err := secretToken()
	if err != nil {
		return nil, "", err
	}
	var result *Player
	err = e.transaction(func() error {
		if err := e.advance(e.clock()); err != nil {
			return err
		}
		if _, ok := e.names[folded]; ok {
			return fail("USERNAME_TAKEN", "用户名已被注册，请换一个")
		}
		if e.RequireBinding && len(bindings) == 0 {
			return fail("INSTANCE_REQUIRED", "开户必须携带当前 AzurPilot 实例身份")
		}
		if len(bindings) > 0 && bindings[0] == nil {
			return fail("INVALID_INSTANCE", "实例身份无效")
		}
		if len(bindings) > 0 {
			if err := e.checkBinding(0, *bindings[0]); err != nil {
				return err
			}
		}
		id := e.state.NextPlayer
		identityCode, err := e.newIdentityCode()
		if err != nil {
			return err
		}
		e.state.NextPlayer++
		now := e.clock().Unix()
		p := &Player{ID: id, Username: name, IdentityCode: identityCode, Folded: folded, PasswordHash: string(hash), UploadHash: hashToken(token), Cash: e.state.Settings.InitialCash, InitialCash: e.state.Settings.InitialCash, Quote: Quote{Price: ap * 100, Previous: ap * 100, ObservedAt: observed, UploadedAt: now}, Positions: map[int64]*Position{}, Orders: []*Order{}, Settlements: []Settlement{}, JoinedAt: now}
		if len(bindings) > 0 {
			b := *bindings[0]
			p.Binding = &b
		}
		e.players[id] = p
		e.dirty[id] = true
		e.points = append(e.points, struct {
			ID    int64
			Point QuotePoint
		}{id, QuotePoint{observed, ap * 100}})
		e.delistIfNeeded(p, time.Unix(now, 0))
		result = clonePlayer(p)
		return nil
	})
	return result, token, err
}
func (e *Engine) Login(username, password string) (*Player, error) {
	_, folded, _ := normalizeName(username)
	e.mu.RLock()
	id := e.names[folded]
	p := e.players[id]
	hash := ""
	if p != nil {
		hash = p.PasswordHash
	}
	e.mu.RUnlock()
	// 不存在的账户也做一次 bcrypt，避免通过耗时枚举用户名。
	if hash == "" {
		hash = "$2a$10$7EqJtq98hPqEX7fNZaFWoO5KOvoM4QVJCloOxDhPvntLSggxnpaVq"
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil || p == nil {
		return nil, fail("LOGIN_FAILED", "用户名或密码错误")
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.players[id].PasswordHash != hash || e.players[id].Folded != folded {
		return nil, fail("LOGIN_FAILED", "账户信息已更新，请重新登录")
	}
	if e.players[id].Disabled {
		return nil, fail("DISABLED", "账户已停用")
	}
	return clonePlayer(e.players[id]), nil
}
func (e *Engine) PlayerExists(id int64) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	p := e.players[id]
	return p != nil && !p.Disabled
}
func (e *Engine) SessionVersion(id int64) uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if p := e.players[id]; p != nil {
		return p.SessionVersion
	}
	return 0
}
func (e *Engine) RotateUpload(id int64) (string, error) {
	token, err := secretToken()
	if err != nil {
		return "", err
	}
	old := ""
	err = e.transaction(func() error {
		p := e.players[id]
		if p == nil {
			return fail("NOT_FOUND", "账户不存在")
		}
		old = p.UploadHash
		e.touch(id).UploadHash = hashToken(token)
		return nil
	})
	if err == nil {
		e.mu.Lock()
		delete(e.uploads, old)
		e.mu.Unlock()
	}
	return token, err
}
func (e *Engine) Upload(token string, ap, observed int64, bindings ...*InstanceBinding) error {
	now := e.clock()
	if err := validQuote(ap, observed, now); err != nil {
		return err
	}
	return e.transaction(func() error {
		if err := e.advance(now); err != nil {
			return err
		}
		id := e.uploads[hashToken(token)]
		owner := e.players[id]
		if owner == nil || owner.Disabled {
			return fail("UNAUTHORIZED", "上传凭据无效")
		}
		if e.RequireBinding && len(bindings) == 0 {
			return fail("INSTANCE_REQUIRED", "报价必须携带绑定实例身份")
		}
		if len(bindings) > 0 {
			b := bindings[0]
			if b == nil || owner.Binding == nil || owner.Binding.Key != b.Key || owner.Binding.InstanceID != b.InstanceID {
				return fail("INSTANCE_MISMATCH", "上传来源与账户绑定实例不一致")
			}
		}
		if observed < owner.Quote.ObservedAt {
			return fail("OLD_QUOTE", "记录时间早于已上传数据")
		}
		if observed == owner.Quote.ObservedAt {
			if owner.Quote.Price == ap*100 {
				return nil
			}
			return fail("OLD_QUOTE", "同一记录时间不能上报不同数值")
		}
		for holder := range e.watchers[id] {
			p := e.touch(holder)
			e.accrue(p, now.Unix())
		}
		p := e.touch(id)
		p.Quote = Quote{Price: ap * 100, Previous: p.Quote.Price, ObservedAt: observed, UploadedAt: now.Unix()}
		e.points = append(e.points, struct {
			ID    int64
			Point QuotePoint
		}{id, QuotePoint{observed, ap * 100}})
		e.delistIfNeeded(p, now)
		for holder := range e.watchers[id] {
			p := e.touch(holder)
			e.risk(p, now)
			e.processOrders(p, now)
		}
		return nil
	})
}
func (e *Engine) borrow(p *Position, now int64) (int64, int64) {
	if p.Quantity >= 0 || p.BorrowAt == 0 || now <= p.BorrowAt {
		return 0, p.BorrowRemainder
	}
	stock := e.players[p.StockID]
	if stock == nil {
		return 0, p.BorrowRemainder
	}
	notional := (-p.Quantity) * stock.Quote.Price
	return carriedInterest(notional, e.state.Settings.Active.BorrowAnnualPPM, now-p.BorrowAt, p.BorrowRemainder)
}
func (e *Engine) accrue(p *Player, now int64) {
	if e.state.Season.Settled {
		return
	}
	now = min(now, e.state.Season.EndsAt)
	for _, pos := range p.Positions {
		if pos.Loan > 0 {
			fee, rem := e.financing(pos, now)
			pos.FinancingRemainder, pos.FinancingAt = rem, now
			if fee > 0 {
				p.Cash -= fee
				f := Fees{Financing: fee}
				p.Fees.Add(f)
				e.state.Season.Revenue.Add(f)
				e.state.LifetimeRevenue.Add(f)
			}
		}
		if pos.Quantity < 0 {
			fee, rem := e.borrow(pos, now)
			pos.BorrowRemainder = rem
			pos.BorrowAt = now
			if fee > 0 {
				p.Cash -= fee
				f := Fees{Borrow: fee}
				p.Fees.Add(f)
				e.state.Season.Revenue.Add(f)
				e.state.LifetimeRevenue.Add(f)
			}
		}
	}
}
func (e *Engine) account(p *Player, now int64) Account {
	a := Account{Player: p, Equity: p.Cash, Available: p.Cash}
	var financedValue, shortMargin int64
	for _, v := range p.Settlements {
		if v.AvailableAt > now {
			a.Unsettled += v.Amount
		}
	}
	for _, pos := range p.Positions {
		price := e.players[pos.StockID].Quote.Price
		a.Equity += pos.Quantity * price
		if pos.Loan > 0 {
			value := pos.Quantity * price
			financedValue += value
			a.FinancingDebt += pos.Loan
			a.Equity -= pos.Loan
			a.LongMargin += capitalMargin(pos, price, e.state.Settings.Active)
			f, _ := e.financing(pos, min(now, e.state.Season.EndsAt))
			if !e.state.Season.Settled {
				a.FinancingAccrued += f
			}
		}
		if pos.Quantity < 0 {
			value := (-pos.Quantity) * price
			a.ShortLiability += value
			shortMargin += capitalMargin(pos, price, e.state.Settings.Active)
			f, _ := e.borrow(pos, min(now, e.state.Season.EndsAt))
			if !e.state.Season.Settled {
				a.BorrowAccrued += f
			}
		}
	}
	a.InitialMargin = shortMargin + a.LongMargin
	a.MaintenanceMargin = rate(a.ShortLiability+financedValue, e.state.Settings.Active.MMRPPM)
	// 融资买入的首付款已从现金扣除，仅冻结价格下跌后的追加保证金差额。
	a.Frozen = a.ShortLiability + shortMargin + max(int64(0), a.LongMargin-(financedValue-a.FinancingDebt))
	for _, o := range p.Orders {
		if o.Status == "pending" {
			a.Frozen += o.Reserved
		}
	}
	a.Equity -= a.BorrowAccrued + a.FinancingAccrued
	a.Available = max(int64(0), p.Cash-a.Unsettled-a.Frozen-a.BorrowAccrued-a.FinancingAccrued)
	return a
}
func (e *Engine) Account(id int64) (Account, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	p := e.players[id]
	if p == nil {
		return Account{}, fail("NOT_FOUND", "账户不存在")
	}
	return e.account(clonePlayer(p), e.clock().Unix()), nil
}
func (e *Engine) ranks(now int64) []Rank {
	out := make([]Rank, 0, len(e.players))
	for _, p := range e.players {
		if !p.Disabled && p.JoinedAt < e.state.Season.EndsAt {
			a := e.account(p, now)
			out = append(out, Rank{PlayerID: p.ID, Username: p.Username, Equity: a.Equity, ReturnPPM: returnPPM(a.Equity, startingCash(p))})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Equity == out[j].Equity {
			return out[i].PlayerID < out[j].PlayerID
		}
		return out[i].Equity > out[j].Equity
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}
func (e *Engine) MarketJSON() ([]byte, string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.clock()
	bucket := now.Unix()
	if e.cache != nil && e.cacheRevision == e.state.Revision && e.cacheTime == bucket && now.Unix() < e.cacheUntil {
		return e.cache, fmt.Sprintf(`"%d-%d-%d"`, e.state.Revision, bucket, e.cacheVersion), nil
	}
	e.cacheUntil = int64(1<<63 - 1)
	open, reason := e.isOpen(now)
	opening, err := e.openingPrices(now, 0)
	if err != nil {
		return nil, "", err
	}
	stocks := make([]Stock, 0, len(e.players))
	for _, p := range e.players {
		stocks = append(stocks, e.marketStock(p, opening[p.ID], now))
		deadline := p.Quote.ObservedAt + e.state.Settings.Active.QuoteTTLSeconds + 1
		if deadline > now.Unix() && deadline < e.cacheUntil {
			e.cacheUntil = deadline
		}
	}
	sort.Slice(stocks, func(i, j int) bool { return stocks[i].ID < stocks[j].ID })
	m := Market{InitialCash: e.state.Settings.InitialCash, DelistThreshold: e.state.Settings.DelistThreshold, Revision: e.state.Revision, ServerTime: now.Unix(), Season: e.state.Season, Rules: e.state.Settings.Active, Open: open, Reason: reason, Stocks: stocks, Rankings: e.ranks(now.Unix()), LifetimeRevenue: e.state.LifetimeRevenue, Participants: len(e.players)}
	b, err := json.Marshal(m)
	e.cache = b
	e.cacheRevision = e.state.Revision
	e.cacheTime = bucket
	e.cacheVersion++
	return b, fmt.Sprintf(`"%d-%d-%d"`, e.state.Revision, bucket, e.cacheVersion), err
}
func (e *Engine) Settings() Settings {
	e.mu.RLock()
	defer e.mu.RUnlock()
	b, _ := json.Marshal(e.state.Settings)
	var s Settings
	_ = json.Unmarshal(b, &s)
	return s
}
func (e *Engine) InitialFunding() int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state.Settings.InitialCash
}
func (e *Engine) SetSettings(s Settings) error {
	if err := ValidateSettings(s); err != nil {
		return err
	}
	return e.transaction(func() error {
		now := e.clock()
		if err := e.advance(now); err != nil {
			return err
		}
		for id, p := range e.players {
			if hasMargin(p) {
				e.accrue(e.touch(id), now.Unix())
			}
		}
		e.state.Settings = s
		for id := range e.players {
			p := e.touch(id)
			e.delistIfNeeded(p, now)
			e.risk(p, now)
		}
		return nil
	})
}
func hasMargin(p *Player) bool {
	for _, v := range p.Positions {
		if v.Quantity < 0 || v.Loan > 0 {
			return true
		}
	}
	return false
}

var clientIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,80}$`)
