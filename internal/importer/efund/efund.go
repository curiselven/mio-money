package efund

import (
	"context"

	"github.com/curiselven/mio-money/internal/domain"
)

// Holding is one data row of 持有信息, with the personal-information rows above
// the header discarded.
type Holding struct {
	Seq              int    `json:"seq"`
	FundCode         string `json:"fund_code"`
	FundName         string `json:"fund_name"`
	ShareClass       string `json:"share_class,omitempty"`
	Manager          string `json:"manager,omitempty"`
	FundAccount      string `json:"fund_account,omitempty"`
	SalesOrg         string `json:"sales_org"`
	TradeAccount     string `json:"trade_account"`
	Quantity         string `json:"quantity"`
	ShareDate        string `json:"share_date"`
	NAV              string `json:"nav"`
	NAVDate          string `json:"nav_date"`
	MarketValueCents int64  `json:"market_value_cents"`
	Currency         string `json:"currency"`
	CurrencyCode     string `json:"currency_code"`
	DividendMethod   string `json:"dividend_method,omitempty"`
}

type Severity string

const (
	SeverityInfo        Severity = "info"
	SeverityWarning     Severity = "warning"
	SeverityBlocksWrite Severity = "blocks_write"
)

type Warning struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Row      int      `json:"row,omitempty"`
}

type Meta struct {
	ShareDate            string `json:"share_date,omitempty"`
	PrintTime            string `json:"print_time,omitempty"`
	TotalRecords         *int   `json:"total_records,omitempty"`
	ActualRows           int    `json:"actual_rows"`
	RecordsMatch         bool   `json:"records_match"`
	NAVDateMismatchFunds int    `json:"nav_date_mismatch_funds"`
}

type Result struct {
	File     string    `json:"file"`
	Meta     Meta      `json:"meta"`
	Holdings []Holding `json:"holdings"`
	Warnings []Warning `json:"warnings"`
}

// ReadRepo is the read-only slice of the store needed by a preview.
type ReadRepo interface {
	ListAccounts(ctx context.Context) ([]domain.Account, error)
	LastConfirmedBefore(ctx context.Context, accountID, date string) (*domain.Snapshot, error)
}

type TotalPreview struct {
	HoldingCount     int    `json:"holding_count"`
	FundCount        int    `json:"fund_count"`
	MarketValueCents int64  `json:"market_value_cents"`
	Currency         string `json:"currency"`
}

type GroupPreview struct {
	Name             string       `json:"name"`
	SalesOrg         string       `json:"sales_org"`
	TradeAccount     string       `json:"trade_account"`
	Target           string       `json:"target,omitempty"`
	AccountID        string       `json:"account_id,omitempty"`
	AccountName      string       `json:"account_name,omitempty"`
	Status           string       `json:"status"`
	NewAccount       bool         `json:"new_account"`
	Unmapped         bool         `json:"unmapped"`
	Ignored          bool         `json:"ignored"`
	FundCount        int          `json:"fund_count"`
	MarketValueCents int64        `json:"market_value_cents"`
	Currency         string       `json:"currency"`
	PreviousAsOf     string       `json:"previous_as_of,omitempty"`
	Diff             *domain.Diff `json:"diff,omitempty"`
	Holdings         []Holding    `json:"holdings"`
}

type UnmappedGroup struct {
	Name             string `json:"name"`
	SalesOrg         string `json:"sales_org"`
	TradeAccount     string `json:"trade_account"`
	FundCount        int    `json:"fund_count"`
	MarketValueCents int64  `json:"market_value_cents"`
	Currency         string `json:"currency"`
}

type Preview struct {
	File               string          `json:"file"`
	Meta               Meta            `json:"meta"`
	Groups             []GroupPreview  `json:"groups"`
	Total              TotalPreview    `json:"total"`
	Unmapped           []UnmappedGroup `json:"unmapped"`
	Warnings           []Warning       `json:"warnings"`
	FileTotalCents     int64           `json:"file_total_cents"`
	MappedTotalCents   int64           `json:"mapped_total_cents"`
	IgnoredTotalCents  int64           `json:"ignored_total_cents"`
	UnmappedTotalCents int64           `json:"unmapped_total_cents"`
}
