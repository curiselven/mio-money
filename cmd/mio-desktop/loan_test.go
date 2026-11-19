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
