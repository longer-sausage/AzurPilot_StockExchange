package exchange

import (
	"errors"
	"time"
)

const InitialCash int64 = 2_000_000_000 // 默认 2 千万模拟币，所有金额以分保存。
const MaxNotional int64 = 100_000_000_000_000
const MaxQuotePrice int64 = 100_000_000
const DefaultDelistThreshold int64 = 500 // 总行动力点数，低于此值时退市。

// 为最坏报价、保证金、费用和强平留足 int64 算术空间，与融券库存无关。
const LedgerSafetyLimit int64 = (1<<63 - 1) / 64

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (f *Failure) Error() string      { return f.Message }
func fail(code, message string) error { return &Failure{code, message} }
func publicError(err error) *Failure {
	var f *Failure
	if errors.As(err, &f) {
		return f
	}
	return &Failure{"INTERNAL", "服务暂不可用，请稍后重试"}
}

type Rules struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	CommissionPPM      int64    `json:"commissionPPM"`
	MinCommission      int64    `json:"minCommission"`
	StampPPM           int64    `json:"stampPPM"`
	StampSide          string   `json:"stampSide"`
	StampRound         int64    `json:"stampRound"`
	LevyPPM            int64    `json:"levyPPM"`
	BorrowAnnualPPM    int64    `json:"borrowAnnualPPM"`
	FinancingAnnualPPM int64    `json:"financingAnnualPPM"`
	IMRPPM             int64    `json:"imrPPM"`
	MMRPPM             int64    `json:"mmrPPM"`
	LotSize            int64    `json:"lotSize"`
	SellDelayDays      int      `json:"sellDelayDays"`
	CashDelayDays      int      `json:"cashDelayDays"`
	Timezone           string   `json:"timezone"`
	Sessions           []string `json:"sessions"`
	WeekdaysOnly       bool     `json:"weekdaysOnly"`
	Holidays           []string `json:"holidays"`
	QuoteTTLSeconds    int64    `json:"quoteTTLSeconds"`
	Source             string   `json:"source"`
}
type Settings struct {
	InitialCash     int64   `json:"initialCash"`
	DelistThreshold int64   `json:"delistThreshold"`
	Active          Rules   `json:"active"`
	Presets         []Rules `json:"presets"`
	StartDay        int     `json:"startDay"`
	EndDaysFromLast int     `json:"endDaysFromLast"`
	Paused          bool    `json:"paused"`
}

func DefaultSettings() Settings {
	base := Rules{ID: "meow", Name: "茗喵全天候", Description: "全天交易，T+0，无印花税；最高 2 倍杠杆、行动力定价、无限模拟流动性。", CommissionPPM: 300, MinCommission: 500, StampSide: "sell", StampRound: 1, BorrowAnnualPPM: 80000, FinancingAnnualPPM: 80000, IMRPPM: 500000, MMRPPM: 300000, LotSize: 1, Timezone: "Asia/Shanghai", Sessions: []string{"00:00-24:00"}, Holidays: []string{}, QuoteTTLSeconds: 86400}
	a := base
	a.ID = "a-share"
	a.Name = "A 股参考"
	a.Description = "百股整手、T+1 可卖、工作日分段交易。无限融券为游戏规则；佣金为可调示例。"
	a.StampPPM = 500
	a.LotSize = 100
	a.SellDelayDays = 1
	a.IMRPPM = 1000000
	a.MMRPPM = 500000
	a.WeekdaysOnly = true
	a.Sessions = []string{"09:30-11:30", "13:00-15:00"}
	a.Source = "https://www.sse.com.cn/lawandrules/sselawsrules2025/stocks/exchange/c/c_20260424_10816482.shtml"
	h := base
	h.ID = "hk"
	h.Name = "港股参考"
	h.Description = "双边印花税 0.1%，整元向上取整，T+2 现金交收；手数与佣金为统一游戏示例。"
	h.CommissionPPM = 300
	h.MinCommission = 3000
	h.StampPPM = 1000
	h.StampSide = "both"
	h.StampRound = 100
	h.LevyPPM = 85
	h.CashDelayDays = 2
	h.LotSize = 100
	h.WeekdaysOnly = true
	h.Sessions = []string{"09:30-12:00", "13:00-16:00"}
	h.Source = "https://www.hkex.com.hk/Services/Rules-and-Forms-and-Fees/Fees/Securities-(Hong-Kong)/Trading/Transaction?sc_lang=en"
	u := base
	u.ID = "us"
	u.Name = "美股参考"
	u.Description = "整股、T+1 现金交收，50% 初始 / 30% 维持做空保证金；佣金为游戏示例。"
	u.CommissionPPM = 0
	u.MinCommission = 100
	u.CashDelayDays = 1
	u.WeekdaysOnly = true
	u.Timezone = "America/New_York"
	u.Sessions = []string{"09:30-16:00"}
	u.Source = "https://www.finra.org/rules-guidance/notices/21-12"
	return Settings{InitialCash: InitialCash, DelistThreshold: DefaultDelistThreshold, Active: base, Presets: []Rules{base, a, h, u}, StartDay: 5, EndDaysFromLast: 4}
}

type Quote struct {
	Price      int64 `json:"price"`
	Previous   int64 `json:"previous"`
	ObservedAt int64 `json:"observedAt"`
	UploadedAt int64 `json:"uploadedAt"`
}
type Position struct {
	StockID            int64 `json:"stockId"`
	Quantity           int64 `json:"quantity"`
	Cost               int64 `json:"cost"`
	Lots               []Lot `json:"lots"`
	BorrowRemainder    int64 `json:"borrowRemainder"`
	BorrowAt           int64 `json:"borrowAt"`
	Loan               int64 `json:"loan"`
	MarginPPM          int64 `json:"marginPPM"`
	MarginUnits        int64 `json:"marginUnits"`
	FinancingAt        int64 `json:"financingAt"`
	FinancingRemainder int64 `json:"financingRemainder"`
}
type Lot struct {
	Quantity    int64 `json:"quantity"`
	AvailableAt int64 `json:"availableAt"`
}
type Settlement struct {
	Amount      int64 `json:"amount"`
	AvailableAt int64 `json:"availableAt"`
}
type Fees struct {
	Commission int64 `json:"commission"`
	Stamp      int64 `json:"stamp"`
	Levy       int64 `json:"levy"`
	Borrow     int64 `json:"borrow"`
	Financing  int64 `json:"financing"`
}

func (f Fees) Total() int64 { return f.Commission + f.Stamp + f.Levy + f.Borrow + f.Financing }
func (f *Fees) Add(g Fees) {
	f.Commission += g.Commission
	f.Stamp += g.Stamp
	f.Levy += g.Levy
	f.Borrow += g.Borrow
	f.Financing += g.Financing
}

type Order struct {
	ID         int64  `json:"id"`
	ClientID   string `json:"clientId"`
	StockID    int64  `json:"stockId"`
	Side       string `json:"side"`
	Kind       string `json:"kind"`
	TIF        string `json:"tif"`
	Quantity   int64  `json:"quantity"`
	Limit      int64  `json:"limit"`
	Status     string `json:"status"`
	Price      int64  `json:"price"`
	Fees       Fees   `json:"fees"`
	CreatedAt  int64  `json:"createdAt"`
	ExecutedAt int64  `json:"executedAt"`
	ExpiresAt  int64  `json:"expiresAt"`
	Reason     string `json:"reason"`
	Reserved   int64  `json:"reserved"`
	Rules      Rules  `json:"rules"`
	Leverage   int64  `json:"leverage"`
}
type Player struct {
	ID             int64               `json:"id"`
	Username       string              `json:"username"`
	IdentityCode   string              `json:"identityCode"`
	SessionVersion uint64              `json:"-"`
	Folded         string              `json:"-"`
	PasswordHash   string              `json:"-"`
	UploadHash     string              `json:"-"`
	Cash           int64               `json:"cash"`
	InitialCash    int64               `json:"initialCash"`
	Quote          Quote               `json:"quote"`
	Positions      map[int64]*Position `json:"positions"`
	Settlements    []Settlement        `json:"settlements"`
	Orders         []*Order            `json:"orders"`
	Realized       int64               `json:"realized"`
	Fees           Fees                `json:"fees"`
	JoinedAt       int64               `json:"joinedAt"`
	Disabled       bool                `json:"disabled"`
	Delisted       bool                `json:"delisted"`
	Binding        *InstanceBinding    `json:"binding"`
}

// 密码与上传凭据仅在数据库中保存哈希，不进入公共 JSON。
type storedPlayer struct {
	Player         *Player `json:"player"`
	Folded         string  `json:"folded"`
	PasswordHash   string  `json:"passwordHash"`
	UploadHash     string  `json:"uploadHash"`
	SessionVersion uint64  `json:"sessionVersion,omitempty"`
}
type Season struct {
	ID       string `json:"id"`
	StartsAt int64  `json:"startsAt"`
	EndsAt   int64  `json:"endsAt"`
	Settled  bool   `json:"settled"`
	Revenue  Fees   `json:"revenue"`
}
type Rank struct {
	Rank      int    `json:"rank"`
	PlayerID  int64  `json:"playerId"`
	Username  string `json:"username"`
	Equity    int64  `json:"equity"`
	ReturnPPM int64  `json:"returnPPM"`
}
type SeasonResult struct {
	Season   Season `json:"season"`
	Rankings []Rank `json:"rankings"`
}
type State struct {
	Settings        Settings `json:"settings"`
	Season          Season   `json:"season"`
	LifetimeRevenue Fees     `json:"lifetimeRevenue"`
	NextPlayer      int64    `json:"nextPlayer"`
	NextOrder       int64    `json:"nextOrder"`
	Revision        uint64   `json:"revision"`
}
type Stock struct {
	ID       int64  `json:"id"`
	Symbol   string `json:"symbol"`
	Username string `json:"username"`
	Quote    Quote  `json:"quote"`
	Stale    bool   `json:"stale"`
	Disabled bool   `json:"disabled"`
	Delisted bool   `json:"delisted"`
	Open     int64  `json:"open"`
}
type Market struct {
	InitialCash     int64   `json:"initialCash"`
	DelistThreshold int64   `json:"delistThreshold"`
	Revision        uint64  `json:"revision"`
	ServerTime      int64   `json:"serverTime"`
	Season          Season  `json:"season"`
	Rules           Rules   `json:"rules"`
	Open            bool    `json:"open"`
	Reason          string  `json:"reason"`
	Stocks          []Stock `json:"stocks"`
	Rankings        []Rank  `json:"rankings"`
	LifetimeRevenue Fees    `json:"lifetimeRevenue"`
	Participants    int     `json:"participants"`
}
type Account struct {
	Player            *Player `json:"player"`
	Equity            int64   `json:"equity"`
	Available         int64   `json:"available"`
	Frozen            int64   `json:"frozen"`
	ShortLiability    int64   `json:"shortLiability"`
	InitialMargin     int64   `json:"initialMargin"`
	MaintenanceMargin int64   `json:"maintenanceMargin"`
	Unsettled         int64   `json:"unsettled"`
	BorrowAccrued     int64   `json:"borrowAccrued"`
	FinancingDebt     int64   `json:"financingDebt"`
	FinancingAccrued  int64   `json:"financingAccrued"`
	LongMargin        int64   `json:"longMargin"`
}
type QuotePoint struct {
	Time  int64 `json:"time"`
	Price int64 `json:"price"`
}
type OrderInput struct {
	ClientID string `json:"clientId"`
	StockID  int64  `json:"stockId"`
	Side     string `json:"side"`
	Kind     string `json:"kind"`
	TIF      string `json:"tif"`
	Quantity int64  `json:"quantity"`
	Limit    int64  `json:"limit"`
	Leverage int64  `json:"leverage"`
}
type Clock func() time.Time
