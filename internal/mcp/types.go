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

type SetLoanTermsInput struct {
	AccountID            string `json:"account_id" jsonschema:"liability account id"`
	AnchorDate           string `json:"anchor_date" jsonschema:"start date of these terms, YYYY-MM-DD"`
	AnchorPrincipalCents int64  `json:"anchor_principal_cents" jsonschema:"remaining principal at anchor_date, integer cents, positive"`
	AnnualRate           string `json:"annual_rate" jsonschema:"annual interest rate as a decimal string, e.g. \"3.10\" for 3.10%; 0-30"`
	Method               string `json:"method" jsonschema:"equal_installment (等额本息) or equal_principal (等额本金)"`
	PaymentDay           int    `json:"payment_day" jsonschema:"monthly payment day, 1-28"`
	RemainingPeriods     int    `json:"remaining_periods" jsonschema:"number of payments remaining after anchor_date"`
}

type GetLoanInput struct {
	AccountID string `json:"account_id" jsonschema:"liability account id"`
	Schedule  bool   `json:"schedule,omitempty" jsonschema:"include every future payment"`
}

type LoanRateChangeInput struct {
	Date          string `json:"date" jsonschema:"effective date, YYYY-MM-DD; applies from the first payment after it"`
	AnnualRate    string `json:"annual_rate" jsonschema:"annual rate as a decimal string, e.g. \"3.20\"; 0-30"`
	PrincipalRule string `json:"principal_rule,omitempty" jsonschema:"equal installment principal in the transition period: new_schedule (default), fixed_payment or old_schedule"`
}

type LoanPrepaymentInput struct {
	Date        string `json:"date" jsonschema:"prepayment date, YYYY-MM-DD"`
	AmountCents int64  `json:"amount_cents" jsonschema:"principal reduction, integer cents"`
	Mode        string `json:"mode" jsonschema:"shorten_periods or reduce_payment"`
}

type SetLoanHistoryInput struct {
	AccountID              string                `json:"account_id" jsonschema:"liability account id"`
	OriginalPrincipalCents int64                 `json:"original_principal_cents" jsonschema:"original loan principal, integer cents"`
	DisbursementDate       string                `json:"disbursement_date,omitempty" jsonschema:"optional disbursement date, YYYY-MM-DD; first period interest uses actual days / 360"`
	FirstPaymentDate       string                `json:"first_payment_date" jsonschema:"date of period 1; its day of month is the payment day"`
	TotalPeriods           int                   `json:"total_periods" jsonschema:"total contracted periods"`
	Method                 string                `json:"method" jsonschema:"equal_installment or equal_principal"`
	InitialAnnualRate      string                `json:"initial_annual_rate" jsonschema:"initial annual rate as a decimal string, e.g. \"4.90\""`
	RateChanges            []LoanRateChangeInput `json:"rate_changes,omitempty" jsonschema:"rate changes, applied from the first payment strictly after each effective date"`
	Prepayments            []LoanPrepaymentInput `json:"prepayments,omitempty" jsonschema:"optional prepayments; shorten periods or reduce payment"`
}

type GetLoanHistoryInput struct {
	AccountID string `json:"account_id" jsonschema:"liability account id"`
	From      int    `json:"from,omitempty" jsonschema:"optional first period number (1-based)"`
	To        int    `json:"to,omitempty" jsonschema:"optional last period number (1-based)"`
	Schedule  bool   `json:"schedule,omitempty" jsonschema:"include every period; otherwise use from/to"`
}

type BankPeriodSampleInput struct {
	Date           string `json:"date" jsonschema:"bank repayment date, YYYY-MM-DD"`
	PaymentCents   int64  `json:"payment_cents" jsonschema:"amount actually deducted, integer cents"`
	PrincipalCents int64  `json:"principal_cents" jsonschema:"principal part, integer cents"`
	InterestCents  int64  `json:"interest_cents" jsonschema:"interest part, integer cents"`
	BalanceCents   *int64 `json:"balance_cents,omitempty" jsonschema:"optional remaining principal after this period, integer cents"`
}

type ReconcileLoanHistoryInput struct {
	AccountID string                  `json:"account_id" jsonschema:"liability account id"`
	Samples   []BankPeriodSampleInput `json:"samples" jsonschema:"bank statements to compare; use periods around rate changes"`
}
