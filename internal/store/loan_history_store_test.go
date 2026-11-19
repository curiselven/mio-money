package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/store"
)

func TestLoanHistoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("file:store_loan_history?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().UTC()
	if err := st.UpsertAccount(ctx, &domain.Account{
		ID: "acc_loan", Name: "示例房贷", Type: domain.AccountLiability, Currency: "CNY",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	history := &domain.LoanHistory{
		AccountID:              "acc_loan",
		OriginalPrincipalCents: 100_000_000,
		DisbursementDate:       "2025-12-15",
		FirstPaymentDate:       "2026-01-20",
		TotalPeriods:           360,
		Method:                 domain.LoanEqualInstallment,
		InitialAnnualRate:      "4.90",
		RateChanges: []domain.LoanRateChange{
			{Date: "2026-01-01", AnnualRate: "4.70"},
			{Date: "2027-06-01", AnnualRate: "3.20"},
		},
		Prepayments: []domain.LoanPrepayment{
			{Date: "2028-03-15", AmountCents: 10_000_000, Mode: domain.PrepayShortenPeriods},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := st.UpsertLoanHistory(ctx, history); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetLoanHistory(ctx, "acc_loan")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.TotalPeriods != 360 || got.DisbursementDate != "2025-12-15" ||
		len(got.RateChanges) != 2 || len(got.Prepayments) != 1 {
		t.Fatalf("history = %+v", got)
	}
	if got.RateChanges[1].AnnualRate != "3.20" || got.Prepayments[0].Mode != domain.PrepayShortenPeriods {
		t.Fatalf("history = %+v", got)
	}
	list, err := st.ListLoanHistories(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v, err = %v", list, err)
	}

	if err := st.DeleteLoanHistory(ctx, "acc_loan"); err != nil {
		t.Fatal(err)
	}
	if got, err := st.GetLoanHistory(ctx, "acc_loan"); err != nil || got != nil {
		t.Fatalf("after delete = %+v, err = %v", got, err)
	}

	terms := &domain.LoanTerms{
		AccountID: "acc_loan", AnchorDate: "2026-01-01", AnchorPrincipalCents: 1_000,
		AnnualRate: "3.10", Method: domain.LoanEqualInstallment, PaymentDay: 20, RemainingPeriods: 12,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := st.UpsertLoan(ctx, terms); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteLoan(ctx, "acc_loan"); err != nil {
		t.Fatal(err)
	}
	if loan, err := st.GetLoan(ctx, "acc_loan"); err != nil || loan != nil {
		t.Fatalf("after delete loan = %+v, err = %v", loan, err)
	}
}
