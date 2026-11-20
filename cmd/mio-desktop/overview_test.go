package main

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/store"
)

func newOverview(t *testing.T) (*OverviewService, *domain.Service) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "mio.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := domain.NewService(st, 0.2)
	return NewOverviewService(svc), svc
}

func submitConfirmed(t *testing.T, svc *domain.Service, accountID, asOf string, value int64) {
	t.Helper()
	result, err := svc.Submit(context.Background(), domain.SubmitInput{
		AccountID: accountID,
		AsOf:      asOf,
		Source:    domain.SourceMCP,
		Items:     []domain.Item{{Name: "测试持仓", Code: "000001", Quantity: "10", MarketValueCents: value}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(context.Background(), result.SnapshotID); err != nil {
		t.Fatal(err)
	}
}

func submitCashConfirmed(t *testing.T, svc *domain.Service, accountID, asOf string, cash int64) {
	t.Helper()
	result, err := svc.Submit(context.Background(), domain.SubmitInput{
		AccountID: accountID,
		AsOf:      asOf,
		Source:    domain.SourceMCP,
		CashCents: &cash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(context.Background(), result.SnapshotID); err != nil {
		t.Fatal(err)
	}
}

func TestOverviewEmpty(t *testing.T) {
	overview, _ := newOverview(t)
	got, err := overview.Get()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Empty || got.NetCents != 0 || len(got.Accounts) != 0 || len(got.History) != 0 {
		t.Fatalf("overview = %+v", got)
	}
	if got.ChangeCents != nil || got.ChangePercent != nil || got.PendingCount != 0 {
		t.Fatalf("overview = %+v", got)
	}
}

func TestOverviewIgnoresUnconfirmedSnapshots(t *testing.T) {
	ctx := context.Background()
	overview, svc := newOverview(t)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "测试账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	submitConfirmed(t, svc, account.ID, "2020-01-01", 100_000)
	submitConfirmed(t, svc, account.ID, "2020-01-02", 110_000)

	pending, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: account.ID, AsOf: "2020-01-03", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "测试持仓", MarketValueCents: 500_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: account.ID, AsOf: "2020-01-04", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "测试持仓", MarketValueCents: 700_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Reject(ctx, rejected.SnapshotID, "测试丢弃"); err != nil {
		t.Fatal(err)
	}

	got, err := overview.Get()
	if err != nil {
		t.Fatal(err)
	}
	if got.NetCents != 110_000 || got.AssetsCents != 110_000 || got.LiabilitiesCents != 0 {
		t.Fatalf("overview = %+v", got)
	}
	if len(got.History) != 2 || got.History[0].ValueCents != 100_000 || got.History[1].ValueCents != 110_000 {
		t.Fatalf("history = %+v", got.History)
	}
	if got.AsOf != "2020-01-02" || len(got.Accounts) != 1 || got.Accounts[0].AsOf != "2020-01-02" {
		t.Fatalf("overview = %+v", got)
	}
	if got.ChangeCents == nil || *got.ChangeCents != 10_000 ||
		got.ChangePercent == nil || math.Abs(*got.ChangePercent-10) > 0.0001 {
		t.Fatalf("change = %v / %v", got.ChangeCents, got.ChangePercent)
	}
	if got.PendingCount != 1 {
		t.Fatalf("pending count = %d, want 1", got.PendingCount)
	}
	if pending.Status != domain.StatusPending {
		t.Fatalf("pending = %+v", pending)
	}
}

func TestOverviewAccountShares(t *testing.T) {
	ctx := context.Background()
	overview, svc := newOverview(t)
	stock, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "股票账户", Type: domain.AccountStock})
	if err != nil {
		t.Fatal(err)
	}
	fund, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "基金账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	liability, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "信用卡", Type: domain.AccountLiability})
	if err != nil {
		t.Fatal(err)
	}
	submitConfirmed(t, svc, stock.ID, "2020-02-01", 300_000)
	submitConfirmed(t, svc, fund.ID, "2020-02-01", 700_000)
	submitCashConfirmed(t, svc, liability.ID, "2020-02-01", -200_000)

	got, err := overview.Get()
	if err != nil {
		t.Fatal(err)
	}
	if got.AssetsCents != 1_000_000 || got.LiabilitiesCents != 200_000 || got.NetCents != 800_000 {
		t.Fatalf("overview = %+v", got)
	}
	if len(got.Accounts) != 3 || got.Accounts[0].Name != "基金账户" {
		t.Fatalf("accounts = %+v", got.Accounts)
	}
	var sum float64
	for _, account := range got.Accounts {
		sum += account.SharePercent
	}
	if math.Abs(sum-100) > 0.01 {
		t.Fatalf("share sum = %v, accounts = %+v", sum, got.Accounts)
	}
	if got.ChangeCents != nil {
		t.Fatalf("single history point must not show a change: %+v", got)
	}
}

func TestOverviewShowsLoanBriefing(t *testing.T) {
	ctx := context.Background()
	overview, svc := newOverview(t)
	loanAccount, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "测试房贷", Type: domain.AccountLiability})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetLoanTerms(ctx, domain.SetLoanTermsInput{
		AccountID: loanAccount.ID, AnchorDate: "2026-01-01", AnchorPrincipalCents: 100_000_000,
		AnnualRate: "3.10", Method: domain.LoanEqualInstallment, PaymentDay: 20, RemainingPeriods: 240,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := overview.Get()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Accounts) != 1 {
		t.Fatalf("accounts = %+v", got.Accounts)
	}
	loan := got.Accounts[0].Loan
	if loan == nil {
		t.Fatal("loan briefing is missing")
	}
	expected, err := svc.Loan(ctx, loanAccount.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if loan.RemainingPeriods != expected.RemainingPeriods ||
		loan.RemainingInterestCents != expected.RemainingInterestCents ||
		loan.BalanceCents != expected.BalanceCents {
		t.Fatalf("loan = %+v, expected = %+v", loan, expected)
	}
	if loan.Terms.AnchorDate != "2026-01-01" || loan.Terms.AnnualRate != "3.10" || loan.Terms.PaymentDay != 20 {
		t.Fatalf("terms for prefilling = %+v", loan.Terms)
	}
	if expected.Next != nil && (loan.NextDate != expected.Next.Date || loan.NextPaymentCents != expected.Next.PaymentCents) {
		t.Fatalf("loan = %+v, expected = %+v", loan, expected)
	}
}

func TestOverviewLiabilityWithoutLoanHasNoBriefing(t *testing.T) {
	ctx := context.Background()
	overview, svc := newOverview(t)
	card, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "信用卡", Type: domain.AccountLiability})
	if err != nil {
		t.Fatal(err)
	}
	submitCashConfirmed(t, svc, card.ID, "2026-01-01", -200_000)

	got, err := overview.Get()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Accounts) != 1 || got.Accounts[0].Loan != nil {
		t.Fatalf("liability without loan terms must have no briefing: %+v", got.Accounts)
	}
}

func TestOverviewNestsPortfoliosUnderAccounts(t *testing.T) {
	ctx := context.Background()
	overview, svc := newOverview(t)
	top, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	portfolio, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例组合", ParentID: top.ID})
	if err != nil {
		t.Fatal(err)
	}
	submitConfirmed(t, svc, portfolio.ID, "2026-11-17", 100_000)

	got, err := overview.Get()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Accounts) != 2 {
		t.Fatalf("accounts = %+v", got.Accounts)
	}
	if got.Accounts[0].AccountID != top.ID || got.Accounts[0].Depth != 0 ||
		got.Accounts[1].AccountID != portfolio.ID || got.Accounts[1].Depth != 1 ||
		got.Accounts[1].ParentID != top.ID {
		t.Fatalf("accounts = %+v", got.Accounts)
	}
}
