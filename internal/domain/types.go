package domain

import "time"

type AccountType string

const (
	AccountCash      AccountType = "cash"
	AccountFund      AccountType = "fund"
	AccountStock     AccountType = "stock"
	AccountMixed     AccountType = "mixed"
	AccountLiability AccountType = "liability"
	AccountOther     AccountType = "other"
)

func (t AccountType) Valid() bool {
	switch t {
	case AccountCash, AccountFund, AccountStock, AccountMixed, AccountLiability, AccountOther:
		return true
	}
	return false
}

type Account struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Platform  string      `json:"platform,omitempty"`
	Type      AccountType `json:"type"`
	Currency  string      `json:"currency"`
	Owner     string      `json:"owner,omitempty"`
	ParentID  string      `json:"parent_id,omitempty"`
	Archived  bool        `json:"archived"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type SnapshotStatus string

const (
	StatusPending   SnapshotStatus = "pending"
	StatusConfirmed SnapshotStatus = "confirmed"
	StatusRejected  SnapshotStatus = "rejected"
	StatusUndone    SnapshotStatus = "undone"
)

type Source string

const (
	SourceScreenshot  Source = "screenshot"
	SourceMCP         Source = "mcp"
	SourceStatement   Source = "statement"
	SourceManual      Source = "manual"
	SourceLoan        Source = "loan_schedule"
	SourceEfundImport Source = "efund_import"
)

func (s Source) Valid() bool {
	switch s {
	case SourceScreenshot, SourceMCP, SourceStatement, SourceManual, SourceLoan, SourceEfundImport:
		return true
	}
	return false
}

type Item struct {
	Name             string `json:"name"`
	Code             string `json:"code,omitempty"`
	Quantity         string `json:"quantity,omitempty"`
	MarketValueCents int64  `json:"market_value_cents"`
	CostCents        *int64 `json:"cost_cents,omitempty"`
	Note             string `json:"note,omitempty"`
}

type Snapshot struct {
	ID          string         `json:"id"`
	AccountID   string         `json:"account_id"`
	AsOf        string         `json:"as_of"`
	Status      SnapshotStatus `json:"status"`
	Source      Source         `json:"source"`
	SourceNote  string         `json:"source_note,omitempty"`
	Currency    string         `json:"currency"`
	CashCents   *int64         `json:"cash_cents"`
	Items       []Item         `json:"items"`
	CreatedAt   time.Time      `json:"created_at"`
	ConfirmedAt *time.Time     `json:"confirmed_at,omitempty"`
}

// TotalCents is cash plus the market value of every item.
func (s *Snapshot) TotalCents() int64 {
	var total int64
	if s.CashCents != nil {
		total += *s.CashCents
	}
	for _, it := range s.Items {
		total += it.MarketValueCents
	}
	return total
}

type AuditEntry struct {
	ID         int64     `json:"id"`
	Action     string    `json:"action"`
	AccountID  string    `json:"account_id"`
	SnapshotID string    `json:"snapshot_id"`
	Detail     string    `json:"detail,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
