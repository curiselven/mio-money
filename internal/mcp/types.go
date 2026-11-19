package mcp

import "github.com/curiselven/mio-money/internal/domain"

type EmptyInput struct{}

type SubmitSnapshotInput struct {
	AccountID  string        `json:"account_id" jsonschema:"account id"`
	AsOf       string        `json:"as_of" jsonschema:"date the data refers to, YYYY-MM-DD"`
	Source     string        `json:"source,omitempty" jsonschema:"screenshot, mcp, statement or manual; defaults to mcp"`
	SourceNote string        `json:"source_note,omitempty" jsonschema:"where the data came from"`
	Currency   string        `json:"currency,omitempty" jsonschema:"snapshot currency; defaults to the account currency"`
	CashCents  *int64        `json:"cash_cents,omitempty" jsonschema:"cash balance in integer cents; negative for liabilities"`
	Items      []domain.Item `json:"items" jsonschema:"positions; an empty list means the position is fully closed"`
}

type SnapshotIDInput struct {
	SnapshotID string `json:"snapshot_id" jsonschema:"snapshot id"`
}

type RejectSnapshotInput struct {
	SnapshotID string `json:"snapshot_id" jsonschema:"snapshot id"`
	Reason     string `json:"reason,omitempty" jsonschema:"why the snapshot is rejected"`
}

type AccountIDInput struct {
	AccountID string `json:"account_id" jsonschema:"account id"`
}

type OptionalAccountIDInput struct {
	AccountID string `json:"account_id,omitempty" jsonschema:"optional account id; omit for all accounts"`
}

type AsOfInput struct {
	AsOf string `json:"as_of,omitempty" jsonschema:"optional YYYY-MM-DD; omit for latest"`
}

type HistoryInput struct {
	AccountID string `json:"account_id,omitempty" jsonschema:"optional account id; omit for net worth history"`
	From      string `json:"from,omitempty" jsonschema:"optional start date YYYY-MM-DD"`
	To        string `json:"to,omitempty" jsonschema:"optional end date YYYY-MM-DD"`
}

type ListAccountsOutput struct {
	Accounts []domain.AccountSummary `json:"accounts"`
}

type HoldingsOutput struct {
	Holdings []domain.Holdings `json:"holdings"`
}

type PendingOutput struct {
	Snapshots []*domain.Snapshot `json:"snapshots"`
}
