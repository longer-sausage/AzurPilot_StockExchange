package exchange

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const MaxPrice int64 = 100_000_000
const LedgerLimit int64 = (1<<63 - 1) / 128

type Clock func() time.Time
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

type TaxBracket struct {
	UpTo    int64 `json:"upTo"`
	RatePPM int64 `json:"ratePPM"`
}
type Settings struct {
	ActiveMarket       MarketRules   `json:"activeMarket"`
	MarketPresets      []MarketRules `json:"marketPresets"`
	StartDay           int           `json:"startDay"`
	EndDaysFromLast    int           `json:"endDaysFromLast"`
	DelistThreshold    int64         `json:"delistThreshold"`
	InitialCash        int64         `json:"initialCash"`
	IssuedShares       int64         `json:"issuedShares"`
	HourlyVolume       int64         `json:"hourlyVolume"`
	BorrowShares       int64         `json:"borrowShares"`
	BorrowHourlyVolume int64         `json:"borrowHourlyVolume"`
	PremiumPPM         int64         `json:"premiumPPM"`
	BorrowAnnualPPM    int64         `json:"borrowAnnualPPM"`
	CommissionPPM      int64         `json:"commissionPPM"`
	MinCommission      int64         `json:"minCommission"`
	QuoteTTLSeconds    int64         `json:"quoteTTLSeconds"`
	TaxBrackets        []TaxBracket  `json:"taxBrackets"`
	Paused             bool          `json:"paused"`
}

func DefaultSettings() Settings {
	return Settings{ActiveMarket: DefaultMarketRules(), MarketPresets: DefaultMarketPresets(), StartDay: 5, EndDaysFromLast: 4, DelistThreshold: 500, InitialCash: 2_000_000_000, IssuedShares: 10000, HourlyVolume: 1000, BorrowShares: 10000, BorrowHourlyVolume: 1000, PremiumPPM: 50000, BorrowAnnualPPM: 80000, CommissionPPM: 300, MinCommission: 500, QuoteTTLSeconds: 86400, TaxBrackets: []TaxBracket{{3_000_000_000, 0}, {8_000_000_000, 80000}, {10_000_000_000, 160000}, {0, 320000}}}
}
func ValidateSettings(s Settings) error {
	if s.StartDay < 1 || s.StartDay > 20 || s.EndDaysFromLast < 0 || s.EndDaysFromLast > 7 || s.DelistThreshold < 0 || s.DelistThreshold > MaxPrice/100 || len(s.MarketPresets) == 0 {
		return fail("INVALID_RULES", "月赛起止或退市阈值无效")
	}
	if err := ValidateMarketRules(s.ActiveMarket); err != nil {
		return err
	}
	if err := ValidateMarketPresets(s.MarketPresets); err != nil {
		return err
	}
	if s.InitialCash < 100 || s.InitialCash > LedgerLimit/16 || s.IssuedShares < 1 || s.BorrowShares < 1 || s.HourlyVolume < 1 || s.BorrowHourlyVolume < 1 || s.IssuedShares > LedgerLimit/MaxPrice || s.BorrowShares > LedgerLimit/MaxPrice || s.HourlyVolume > LedgerLimit/MaxPrice || s.BorrowHourlyVolume > LedgerLimit/MaxPrice || s.PremiumPPM < 0 || s.PremiumPPM > 1_000_000 || s.BorrowAnnualPPM < 0 || s.BorrowAnnualPPM > 1_000_000 || s.CommissionPPM < 0 || s.CommissionPPM > 1_000_000 || s.MinCommission < 0 || s.MinCommission > MaxPrice || s.QuoteTTLSeconds < 60 || s.QuoteTTLSeconds > 604800 || len(s.TaxBrackets) == 0 {
		return fail("INVALID_RULES", "交易规则超出整数安全范围")
	}
	last := int64(-1)
	for i, b := range s.TaxBrackets {
		if b.RatePPM < 0 || b.RatePPM > 1_000_000 || (i == len(s.TaxBrackets)-1 && b.UpTo != 0) || (i < len(s.TaxBrackets)-1 && b.UpTo <= last) {
			return fail("INVALID_RULES", "税率档须递增且最后一档 upTo 为 0")
		}
		last = b.UpTo
	}
	return nil
}

type Position struct {
	ShortCost          int64 `json:"shortCost"`
	Loan               int64 `json:"loan"`
	FinancingAt        int64 `json:"financingAt"`
	FinancingRemainder int64 `json:"financingRemainder"`
	Lots               []Lot `json:"lots"`
	StockID            int64 `json:"stockId"`
	Quantity           int64 `json:"quantity"`
	Cost               int64 `json:"cost"`
	ShortQuantity      int64 `json:"shortQuantity"`
	ShortProceeds      int64 `json:"shortProceeds"`
	Collateral         int64 `json:"collateral"`
	BorrowAt           int64 `json:"borrowAt"`
	BorrowRemainder    int64 `json:"borrowRemainder"`
}
type Account struct {
	MetricsSince         int64               `json:"metricsSince"`
	LegacyMetrics        bool                `json:"legacyMetrics"`
	IdentityCode         string              `json:"identityCode"`
	Watchlist            []int64             `json:"watchlist"`
	Realized             int64               `json:"realized"`
	Fees                 Revenue             `json:"fees"`
	Settlements          []Settlement        `json:"settlements"`
	InitialCash          int64               `json:"initialCash"`
	ID                   int64               `json:"id"`
	Revision             uint64              `json:"revision"`
	Username             string              `json:"username"`
	OfficialUserID       string              `json:"officialUserId"`
	PasswordHash         string              `json:"passwordHash,omitempty"`
	PasswordVersion      int64               `json:"passwordVersion"`
	ForcePasswordChange  bool                `json:"forcePasswordChange"`
	CredentialsDelivered bool                `json:"credentialsDelivered"`
	Cash                 int64               `json:"cash"`
	Reserved             int64               `json:"reserved"`
	BoundStockID         int64               `json:"boundStockId"`
	Disabled             bool                `json:"disabled"`
	Positions            map[int64]*Position `json:"positions"`
	CreatedAt            int64               `json:"createdAt"`
}
type Stock struct {
	HistoryCheckedAt      int64  `json:"historyCheckedAt"`
	CheckedHistoryVersion uint64 `json:"checkedHistoryVersion"`
	DeletedAt             int64  `json:"deletedAt,omitempty"`
	Disabled              bool   `json:"disabled"`
	UploadedAt            int64  `json:"uploadedAt"`
	Open                  int64  `json:"open"`
	OpenAt                int64  `json:"openAt"`
	OpenDay               string `json:"openDay"`
	TradeVersion          uint64 `json:"tradeVersion"`
	ID                    int64  `json:"id"`
	Revision              uint64 `json:"revision"`
	Symbol                string `json:"symbol"`
	Name                  string `json:"name"`
	InstanceID            string `json:"instanceId,omitempty"`
	PublicKey             string `json:"publicKey,omitempty"`
	BindingCode           string `json:"bindingCode,omitempty"`
	AccountID             int64  `json:"accountId,omitempty"`
	Price                 int64  `json:"price"`
	Previous              int64  `json:"previous"`
	ObservedAt            int64  `json:"observedAt"`
	Cursor                int64  `json:"cursor"`
	HistoryVersion        uint64 `json:"historyVersion"`
	IssuedShares          int64  `json:"issuedShares"`
	HourlyVolume          int64  `json:"hourlyVolume"`
	BorrowShares          int64  `json:"borrowShares"`
	BorrowHourlyVolume    int64  `json:"borrowHourlyVolume"`
	Outstanding           int64  `json:"outstanding"`
	Borrowed              int64  `json:"borrowed"`
	Hour                  int64  `json:"hour"`
	NormalUsed            int64  `json:"normalUsed"`
	BorrowUsed            int64  `json:"borrowUsed"`
	Delisted              bool   `json:"delisted"`
}
type Order struct {
	LegacyFilled    int64       `json:"legacyFilled,omitempty"`
	LegacyNotional  int64       `json:"legacyNotional,omitempty"`
	LegacyUpdatedAt int64       `json:"legacyUpdatedAt,omitempty"`
	TradeRecorded   bool        `json:"tradeRecorded"`
	Kind            string      `json:"kind"`
	TimeInForce     string      `json:"timeInForce"`
	ExpiresAt       int64       `json:"expiresAt"`
	Triggered       bool        `json:"triggered"`
	GameRules       GameRules   `json:"gameRules"`
	Leverage        int64       `json:"leverage"`
	Rules           MarketRules `json:"rules"`
	Stamp           int64       `json:"stamp"`
	Levy            int64       `json:"levy"`
	ID              int64       `json:"id"`
	AccountID       int64       `json:"accountId"`
	ClientID        string      `json:"clientId"`
	PreviewID       string      `json:"previewId"`
	StockID         int64       `json:"stockId"`
	Side            string      `json:"side"`
	Quantity        int64       `json:"quantity"`
	Remaining       int64       `json:"remaining"`
	Filled          int64       `json:"filled"`
	Price           int64       `json:"price"`
	AveragePrice    int64       `json:"averagePrice"`
	Notional        int64       `json:"notional"`
	Tax             int64       `json:"tax"`
	Commission      int64       `json:"commission"`
	TaxRatePPM      int64       `json:"taxRatePPM"`
	Status          string      `json:"status"`
	Reserved        int64       `json:"reserved"`
	CreatedAt       int64       `json:"createdAt"`
	UpdatedAt       int64       `json:"updatedAt"`
	EligibleHour    int64       `json:"eligibleHour"`
	Reason          string      `json:"reason,omitempty"`
}
type OrderInput struct {
	Kind           string `json:"kind,omitempty"`
	TimeInForce    string `json:"timeInForce,omitempty"`
	Leverage       int64  `json:"leverage,omitempty"`
	OfficialUserID string `json:"officialUserId"`
	Stock          string `json:"stock"`
	Side           string `json:"side"`
	Quantity       int64  `json:"quantity"`
	Price          int64  `json:"price"`
	ClientID       string `json:"clientId"`
}
type Preview struct {
	Kind           string         `json:"kind"`
	TimeInForce    string         `json:"timeInForce"`
	Repayment      int64          `json:"repayment"`
	GameRules      GameRules      `json:"-"`
	Financing      int64          `json:"financing"`
	Collateral     int64          `json:"collateral"`
	Leverage       int64          `json:"leverage"`
	Stamp          int64          `json:"stamp"`
	Levy           int64          `json:"levy"`
	Rules          MarketRules    `json:"rules"`
	PreviewID      string         `json:"previewId"`
	ExpiresAt      int64          `json:"expiresAt"`
	Stock          map[string]any `json:"stock"`
	Side           string         `json:"side"`
	Quantity       int64          `json:"quantity"`
	Price          int64          `json:"price"`
	Notional       int64          `json:"notional"`
	TaxRatePPM     int64          `json:"taxRatePPM"`
	Tax            int64          `json:"tax"`
	Commission     int64          `json:"commission"`
	EstimatedTotal int64          `json:"estimatedTotal"`
	PremiumPPM     int64          `json:"premiumPPM"`
	AccountID      int64          `json:"-"`
	Input          OrderInput     `json:"-"`
}
type GameRules struct {
	TaxBrackets []TaxBracket `json:"taxBrackets"`
	PremiumPPM  int64        `json:"premiumPPM"`
}
type Point struct {
	T int64 `json:"t"`
	P int64 `json:"p"`
}
type Candle struct {
	T int64 `json:"t"`
	O int64 `json:"o"`
	H int64 `json:"h"`
	L int64 `json:"l"`
	C int64 `json:"c"`
	N int64 `json:"n"`
}
type Month struct {
	Month  string `json:"month"`
	Count  int64  `json:"count"`
	Digest string `json:"digest"`
}
type Meta struct {
	FeatureVersion     int      `json:"featureVersion"`
	Revenue            Revenue  `json:"revenue"`
	LifetimeRevenue    Revenue  `json:"lifetimeRevenue"`
	NextTrade          int64    `json:"nextTrade"`
	DelistCheckedStart int64    `json:"delistCheckedStart"`
	Season             Season   `json:"season"`
	Version            uint64   `json:"version"`
	MarketVersion      uint64   `json:"marketVersion"`
	NextAccount        int64    `json:"nextAccount"`
	NextStock          int64    `json:"nextStock"`
	NextOrder          int64    `json:"nextOrder"`
	AuctionHour        int64    `json:"auctionHour"`
	AuctionOpen        bool     `json:"auctionOpen"`
	Settings           Settings `json:"settings"`
}
type Lot struct {
	Quantity    int64 `json:"quantity"`
	AvailableAt int64 `json:"availableAt"`
}
type Settlement struct {
	Amount      int64 `json:"amount"`
	AvailableAt int64 `json:"availableAt"`
}
type Season struct {
	ID       string `json:"id"`
	StartsAt int64  `json:"startsAt"`
	EndsAt   int64  `json:"endsAt"`
	Settled  bool   `json:"settled"`
}
type SeasonRank struct {
	AccountID int64  `json:"accountId"`
	Username  string `json:"username"`
	Equity    int64  `json:"equity"`
	ReturnPPM int64  `json:"returnPPM"`
	Rank      int    `json:"rank"`
}
type SeasonArchive struct {
	Revenue  Revenue      `json:"revenue"`
	Season   Season       `json:"season"`
	Rankings []SeasonRank `json:"rankings"`
}
type Entity struct {
	Kind   string
	ID     string
	Data   []byte
	Delete bool
}
type HistoryWrite struct {
	StockID int64
	Points  []Point
}
type Store interface {
	Load() ([]Entity, error)
	Commit([]Entity, []HistoryWrite) error
	Chart(int64, int64, int64, int64) ([]Candle, error)
	Manifest(int64) ([]Month, error)
	Close() error
}

func encode(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
func idstr(id int64) string { return fmt.Sprint(id) }
func rate(amount, ppm int64) int64 {
	return amount/1_000_000*ppm + (amount%1_000_000*ppm+999999)/1_000_000
}
func validQQ(qq string) bool {
	if len(qq) < 5 || len(qq) > 16 || qq[0] == '0' {
		return false
	}
	for _, c := range qq {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func normalized(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
