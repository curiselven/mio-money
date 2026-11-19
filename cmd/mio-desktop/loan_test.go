package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/store"
)

func newLoanTest(t *testing.T) (*LoanService, *domain.Service, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "mio.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := domain.NewService(st, 0.2)
	return NewLoanService(svc), svc, st
}

func loanForm(accountID string) LoanFormInput {
	return LoanFormInput{
		AccountID:            accountID,
		AnchorDate:           "2026-01-01",
		AnchorPrincipalCents: 100_000_000,
		AnnualRate:           "3.10",
		Method:               "equal_installment",
		PaymentDay:           20,
		RemainingPeriods:     240,
	}
}

func TestLoanPreviewDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	loans, svc, st := newLoanTest(t)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "测试房贷", Type: domain.AccountLiability})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := loans.Preview(loanForm(account.ID))
	if err != nil {
		t.Fatal(err)
	}
	if preview.PaymentCents != 559_617 || preview.LastPaymentCents != 559_577 {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.NextDate != "2026-01-20" || preview.LastDate != "2045-12-20" {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.RemainingInterestCents != 34_308_040 {
		t.Fatalf("remaining interest = %d", preview.RemainingInterestCents)
	}
	if stored, err := st.GetLoan(ctx, account.ID); err != nil || stored != nil {
		t.Fatalf("preview must not store terms: %+v, err = %v", stored, err)
	}
	if confirmed, err := st.ListConfirmed(ctx, account.ID, "", ""); err != nil || len(confirmed) != 0 {
		t.Fatalf("preview must not write snapshots: %d, err = %v", len(confirmed), err)
	}

	invalid := loanForm(account.ID)
	invalid.PaymentDay = 29
	if _, err := loans.Preview(invalid); err == nil {
		t.Fatal("payment day 29 must fail")
	}
}

func TestLoanSaveWritesTermsAuditAndSync(t *testing.T) {
	ctx := context.Background()
	loans, svc, st := newLoanTest(t)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "测试房贷", Type: domain.AccountLiability})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := loans.Save(loanForm(account.ID))
	if err != nil {
		t.Fatal(err)
	}
	if saved.AccountID != account.ID || saved.RemainingPeriods != 240 {
		t.Fatalf("saved = %+v", saved)
	}
	stored, err := st.GetLoan(ctx, account.ID)
	if err != nil || stored == nil || stored.AnnualRate != "3.10" {
		t.Fatalf("stored = %+v, err = %v", stored, err)
	}
	confirmed, err := st.ListConfirmed(ctx, account.ID, "", "")
	if err != nil || len(confirmed) < 2 {
		t.Fatalf("sync created %d snapshots, err = %v", len(confirmed), err)
	}
	audit, err := st.ListAudit(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range audit {
		if entry.Action == "loan_terms" && strings.Contains(entry.Detail, "来源 desktop") {
			found = true
		}
	}
	if !found {
		t.Fatalf("desktop audit entry missing: %+v", audit)
	}
}

func historyFormInput(accountID string) LoanHistoryFormInput {
	return LoanHistoryFormInput{
		AccountID:              accountID,
		OriginalPrincipalCents: 100_000_000,
		FirstPaymentDate:       "2026-01-20",
		TotalPeriods:           12,
		Method:                 "equal_installment",
		InitialAnnualRate:      "4.90",
		RateChanges:            []domain.LoanRateChange{{Date: "2026-06-25", AnnualRate: "5.00"}},
	}
}

func TestLoanHistoryPreviewDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	loans, svc, st := newLoanTest(t)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "测试房贷", Type: domain.AccountLiability})
	if err != nil {
		t.Fatal(err)
	}
	view, err := loans.PreviewHistory(historyFormInput(account.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Periods) != 12 || view.PaidPeriods == 0 || view.Next == nil {
		t.Fatalf("view = %+v", view.LoanCalculation)
	}
	if view.Periods[0].PaymentCents != 8_556_166 {
		t.Fatalf("first payment = %d", view.Periods[0].PaymentCents)
	}
	if stored, err := st.GetLoanHistory(ctx, account.ID); err != nil || stored != nil {
		t.Fatalf("preview must not store history: %+v, err = %v", stored, err)
	}
	if confirmed, err := st.ListConfirmed(ctx, account.ID, "", ""); err != nil || len(confirmed) != 0 {
		t.Fatalf("preview must not write snapshots: %d, err = %v", len(confirmed), err)
	}

	invalid := historyFormInput(account.ID)
	invalid.TotalPeriods = 0
	if _, err := loans.PreviewHistory(invalid); err == nil {
		t.Fatal("zero periods must fail")
	}
}

func TestLoanHistorySaveReplacesAnchor(t *testing.T) {
	ctx := context.Background()
	loans, svc, st := newLoanTest(t)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "测试房贷", Type: domain.AccountLiability})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loans.Save(loanForm(account.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := loans.SaveHistory(historyFormInput(account.ID)); err != nil {
		t.Fatal(err)
	}
	if terms, err := st.GetLoan(ctx, account.ID); err != nil || terms != nil {
		t.Fatalf("anchor terms should be gone: %+v, err = %v", terms, err)
	}
	history, err := st.GetLoanHistory(ctx, account.ID)
	if err != nil || history == nil || history.TotalPeriods != 12 {
		t.Fatalf("history = %+v, err = %v", history, err)
	}
	if confirmed, err := st.ListConfirmed(ctx, account.ID, "", ""); err != nil || len(confirmed) < 2 {
		t.Fatalf("snapshots = %d, err = %v", len(confirmed), err)
	}
	audit, err := st.ListAudit(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range audit {
		if entry.Action == "loan_history" && strings.Contains(entry.Detail, "来源 desktop") {
			found = true
		}
	}
	if !found {
		t.Fatalf("desktop history audit missing: %+v", audit)
	}

	overview, err := NewOverviewService(svc).Get()
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Accounts) != 1 || overview.Accounts[0].Loan == nil || overview.Accounts[0].Loan.History == nil {
		t.Fatalf("overview = %+v", overview.Accounts)
	}
}

func TestLoanHistoryDisbursementAndPaidTotals(t *testing.T) {
	ctx := context.Background()
	loans, svc, st := newLoanTest(t)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "测试房贷", Type: domain.AccountLiability})
	if err != nil {
		t.Fatal(err)
	}
	form := historyFormInput(account.ID)
	form.DisbursementDate = "2026-01-01"
	view, err := loans.PreviewHistory(form)
	if err != nil {
		t.Fatal(err)
	}
	// 19 days: 100000000 * 49/12000 * 19/30 = 258611.11 -> 258611.
	if view.Periods[0].InterestCents != 258_611 {
		t.Fatalf("first interest = %d", view.Periods[0].InterestCents)
	}
	if view.PaidPeriods == 0 || view.PaidTotalCents != view.PaidPrincipalCents+view.PaidInterestCents {
		t.Fatalf("paid totals = %+v", view.LoanCalculation)
	}
	if _, err := loans.SaveHistory(form); err != nil {
		t.Fatal(err)
	}
	history, err := st.GetLoanHistory(ctx, account.ID)
	if err != nil || history == nil || history.DisbursementDate != "2026-01-01" {
		t.Fatalf("history = %+v, err = %v", history, err)
	}
	view, err = svc.LoanHistory(ctx, account.ID)
	if err != nil || view.PrepaidCents != 0 || view.PaidTotalCents != view.PaidPrincipalCents+view.PaidInterestCents {
		t.Fatalf("saved view = %+v, err = %v", view, err)
	}
}
