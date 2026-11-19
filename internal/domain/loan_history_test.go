package domain_test

import (
	"context"
	"strings"
	"testing"

	"github.com/curiselven/mio-money/internal/domain"
)

func testHistory() domain.LoanHistory {
	return domain.LoanHistory{
		AccountID:              "acc_loan",
		OriginalPrincipalCents: 100_000_000,
		FirstPaymentDate:       "2026-01-20",
		TotalPeriods:           12,
		Method:                 domain.LoanEqualInstallment,
		InitialAnnualRate:      "4.90",
		RateChanges:            []domain.LoanRateChange{},
		Prepayments:            []domain.LoanPrepayment{},
	}
}

func TestHistoryKnownMonthlyPayment(t *testing.T) {
	h := testHistory()
	h.TotalPeriods = 360
	periods, err := domain.LoanHistorySchedule(h)
	if err != nil {
		t.Fatal(err)
	}
	if periods[0].PaymentCents != 530_727 {
		t.Fatalf("first payment = %d, want 530727", periods[0].PaymentCents)
	}
	var principal int64
	for _, p := range periods {
		principal += p.PrincipalCents
		if p.BalanceCents < 0 {
			t.Fatalf("negative balance: %+v", p)
		}
	}
	if principal != h.OriginalPrincipalCents || periods[len(periods)-1].BalanceCents != 0 {
		t.Fatalf("principal = %d, last = %+v", principal, periods[len(periods)-1])
	}
}

func TestHistoryRateChangeRecomputesPayment(t *testing.T) {
	h := testHistory()
	h.RateChanges = []domain.LoanRateChange{{Date: "2026-06-25", AnnualRate: "5.00"}}
	periods, err := domain.LoanHistorySchedule(h)
	if err != nil {
		t.Fatal(err)
	}
	if periods[5].Date != "2026-06-20" || periods[5].AnnualRate != "4.90" {
		t.Fatalf("period 6 = %+v", periods[5])
	}
	if periods[6].Date != "2026-07-20" || periods[6].AnnualRate != "5.00" {
		t.Fatalf("period 7 = %+v", periods[6])
	}
	if periods[5].PaymentCents == periods[6].PaymentCents {
		t.Fatalf("payment should be recomputed: %+v / %+v", periods[5], periods[6])
	}
	for i := 6; i < 11; i++ {
		if periods[i].PaymentCents != periods[6].PaymentCents {
			t.Fatalf("payment should stay fixed after the change: %+v", periods[i])
		}
	}
	if periods[len(periods)-1].BalanceCents != 0 {
		t.Fatalf("last = %+v", periods[len(periods)-1])
	}
}

func TestHistoryRateChangeOnPaymentDateStartsNextPeriod(t *testing.T) {
	h := testHistory()
	h.RateChanges = []domain.LoanRateChange{{Date: "2026-06-20", AnnualRate: "5.00"}}
	periods, err := domain.LoanHistorySchedule(h)
	if err != nil {
		t.Fatal(err)
	}
	if periods[5].AnnualRate != "4.90" || periods[6].AnnualRate != "5.00" {
		t.Fatalf("periods = %+v / %+v", periods[5], periods[6])
	}
}

func TestHistoryRateChangeBetweenPaymentsStartsAtNextPayment(t *testing.T) {
	h := testHistory()
	h.RateChanges = []domain.LoanRateChange{{Date: "2026-06-05", AnnualRate: "5.00"}}
	periods, err := domain.LoanHistorySchedule(h)
	if err != nil {
		t.Fatal(err)
	}
	if periods[4].Date != "2026-05-20" || periods[4].AnnualRate != "4.90" {
		t.Fatalf("period 5 = %+v", periods[4])
	}
	if periods[5].Date != "2026-06-20" || periods[5].AnnualRate != "5.00" {
		t.Fatalf("period 6 = %+v", periods[5])
	}
}

func TestHistoryMultipleRateChanges(t *testing.T) {
	h := testHistory()
	h.RateChanges = []domain.LoanRateChange{
		{Date: "2026-04-01", AnnualRate: "5.00"},
		{Date: "2026-08-15", AnnualRate: "5.50"},
	}
	periods, err := domain.LoanHistorySchedule(h)
	if err != nil {
		t.Fatal(err)
	}
	if periods[2].AnnualRate != "4.90" || periods[3].AnnualRate != "5.00" || periods[7].AnnualRate != "5.50" {
		t.Fatalf("rates = %+v / %+v / %+v", periods[2], periods[3], periods[7])
	}
	for i := 3; i < 7; i++ {
		if periods[i].PaymentCents != periods[3].PaymentCents {
			t.Fatalf("payment should stay fixed between changes: %+v", periods[i])
		}
	}
	if periods[7].PaymentCents == periods[6].PaymentCents {
		t.Fatalf("payment should change at the second adjustment: %+v / %+v", periods[6], periods[7])
	}
	if periods[len(periods)-1].BalanceCents != 0 {
		t.Fatalf("last = %+v", periods[len(periods)-1])
	}
}

func TestHistoryEqualPrincipal(t *testing.T) {
	h := testHistory()
	h.Method = domain.LoanEqualPrincipal
	h.RateChanges = []domain.LoanRateChange{{Date: "2026-07-01", AnnualRate: "3.20"}}
	periods, err := domain.LoanHistorySchedule(h)
	if err != nil {
		t.Fatal(err)
	}
	if periods[0].PrincipalCents != 8_333_333 {
		t.Fatalf("first principal = %d", periods[0].PrincipalCents)
	}
	var principal int64
	for _, p := range periods {
		principal += p.PrincipalCents
		if p.PaymentCents != p.PrincipalCents+p.InterestCents {
			t.Fatalf("payment mismatch: %+v", p)
		}
	}
	if principal != h.OriginalPrincipalCents || periods[len(periods)-1].BalanceCents != 0 {
		t.Fatalf("principal = %d, last = %+v", principal, periods[len(periods)-1])
	}
}

func TestHistoryPrepaymentShortenPeriods(t *testing.T) {
	h := testHistory()
	h.Prepayments = []domain.LoanPrepayment{
		{Date: "2026-06-25", AmountCents: 30_000_000, Mode: domain.PrepayShortenPeriods},
	}
	periods, err := domain.LoanHistorySchedule(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(periods) >= h.TotalPeriods {
		t.Fatalf("periods = %d, want fewer than %d", len(periods), h.TotalPeriods)
	}
	var principal, prepaid int64
	for _, p := range periods {
		principal += p.PrincipalCents
	}
	prepaid = 30_000_000
	if principal+prepaid != h.OriginalPrincipalCents {
		t.Fatalf("principal = %d + prepaid = %d", principal, prepaid)
	}
	if periods[len(periods)-1].BalanceCents != 0 {
		t.Fatalf("last = %+v", periods[len(periods)-1])
	}
}

func TestHistoryPrepaymentReducePayment(t *testing.T) {
	h := testHistory()
	h.Prepayments = []domain.LoanPrepayment{
		{Date: "2026-06-25", AmountCents: 30_000_000, Mode: domain.PrepayReducePayment},
	}
	periods, err := domain.LoanHistorySchedule(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(periods) != h.TotalPeriods {
		t.Fatalf("periods = %d", len(periods))
	}
	if periods[6].PaymentCents >= periods[5].PaymentCents {
		t.Fatalf("payment should fall after prepayment: %+v / %+v", periods[5], periods[6])
	}
	var principal int64
	for _, p := range periods {
		principal += p.PrincipalCents
	}
	if principal+30_000_000 != h.OriginalPrincipalCents {
		t.Fatalf("principal = %d", principal)
	}
}

func TestHistoryAt(t *testing.T) {
	h := testHistory()
	view, err := domain.LoanHistoryAt(h, "2026-06-20")
	if err != nil {
		t.Fatal(err)
	}
	if view.PaidPeriods != 6 || view.RemainingPeriods != 6 {
		t.Fatalf("state = %+v", view.LoanCalculation)
	}
	if view.BalanceCents != view.Periods[5].BalanceCents {
		t.Fatalf("balance = %d", view.BalanceCents)
	}
	if view.Next == nil || view.Next.Date != "2026-07-20" {
		t.Fatalf("next = %+v", view.Next)
	}
	if view.RemainingInterestCents <= 0 {
		t.Fatalf("remaining interest = %d", view.RemainingInterestCents)
	}

	done, err := domain.LoanHistoryAt(h, "2027-01-20")
	if err != nil {
		t.Fatal(err)
	}
	if done.BalanceCents != 0 || done.Next != nil || done.RemainingPeriods != 0 || done.RemainingInterestCents != 0 {
		t.Fatalf("done = %+v", done.LoanCalculation)
	}
}

func TestHistoryPaymentDayClampsToMonthEnd(t *testing.T) {
	h := testHistory()
	h.FirstPaymentDate = "2026-01-31"
	periods, err := domain.LoanHistorySchedule(h)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30"}
	for i, date := range want {
		if periods[i].Date != date {
			t.Fatalf("period %d = %s, want %s", i+1, periods[i].Date, date)
		}
	}
}

func TestHistoryValidation(t *testing.T) {
	cases := []struct {
		name   string
		change func(*domain.LoanHistory)
	}{
		{"principal zero", func(h *domain.LoanHistory) { h.OriginalPrincipalCents = 0 }},
		{"periods zero", func(h *domain.LoanHistory) { h.TotalPeriods = 0 }},
		{"periods too many", func(h *domain.LoanHistory) { h.TotalPeriods = 2000 }},
		{"bad first date", func(h *domain.LoanHistory) { h.FirstPaymentDate = "2026-02-30" }},
		{"bad method", func(h *domain.LoanHistory) { h.Method = "interest_only" }},
		{"rate too high", func(h *domain.LoanHistory) { h.InitialAnnualRate = "31" }},
		{"duplicate change date", func(h *domain.LoanHistory) {
			h.RateChanges = []domain.LoanRateChange{{Date: "2026-03-01", AnnualRate: "4.5"}, {Date: "2026-03-01", AnnualRate: "4.6"}}
		}},
		{"bad prepay mode", func(h *domain.LoanHistory) {
			h.Prepayments = []domain.LoanPrepayment{{Date: "2026-06-01", AmountCents: 100, Mode: "magic"}}
		}},
		{"prepay after last date", func(h *domain.LoanHistory) {
			h.Prepayments = []domain.LoanPrepayment{{Date: "2027-01-20", AmountCents: 100, Mode: domain.PrepayShortenPeriods}}
		}},
	}
	for _, tc := range cases {
		h := testHistory()
		tc.change(&h)
		if _, err := domain.LoanHistorySchedule(h); err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
	}
}

func historyInput(accountID string) domain.SetLoanHistoryInput {
	return domain.SetLoanHistoryInput{
		AccountID:              accountID,
		OriginalPrincipalCents: 100_000_000,
		FirstPaymentDate:       "2026-01-20",
		TotalPeriods:           12,
		Method:                 domain.LoanEqualInstallment,
		InitialAnnualRate:      "4.90",
		RateChanges:            []domain.LoanRateChange{{Date: "2026-06-25", AnnualRate: "5.00"}},
		Prepayments:            []domain.LoanPrepayment{},
		Origin:                 "desktop",
	}
}

func TestSetLoanHistorySwitchesMode(t *testing.T) {
	ctx := context.Background()
	_, st := newTestServiceAndStore(t)
	svc := domain.NewService(st, 0.2)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "测试房贷", Type: domain.AccountLiability})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetLoanTerms(ctx, domain.SetLoanTermsInput{
		AccountID: account.ID, AnchorDate: "2026-01-01", AnchorPrincipalCents: 100_000_000,
		AnnualRate: "4.90", Method: domain.LoanEqualInstallment, PaymentDay: 20, RemainingPeriods: 240,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetLoanHistory(ctx, historyInput(account.ID)); err != nil {
		t.Fatal(err)
	}
	if loan, err := st.GetLoan(ctx, account.ID); err != nil || loan != nil {
		t.Fatalf("anchor terms should be removed: %+v, err = %v", loan, err)
	}
	if history, err := st.GetLoanHistory(ctx, account.ID); err != nil || history == nil {
		t.Fatalf("history = %+v, err = %v", history, err)
	}
	if _, err := svc.Loan(ctx, account.ID, false); err == nil || !strings.Contains(err.Error(), "get_loan_history") {
		t.Fatalf("get_loan should point at the history tool: %v", err)
	}
	view, err := svc.LoanHistory(ctx, account.ID)
	if err != nil || view.History.TotalPeriods != 12 || view.Next == nil {
		t.Fatalf("view = %+v, err = %v", view, err)
	}
	briefings, err := svc.LoanBriefings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	briefing := briefings[account.ID]
	if briefing.History == nil || briefing.BalanceCents != view.BalanceCents || briefing.RemainingPeriods != view.RemainingPeriods {
		t.Fatalf("briefing = %+v, view = %+v", briefing, view.LoanCalculation)
	}

	result, err := svc.SyncLoans(ctx, "2026-03-25", account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 3 || len(result.Accounts) != 1 || result.Accounts[0].RemainingPeriods != 9 {
		t.Fatalf("sync = %+v", result)
	}
	again, err := svc.SyncLoans(ctx, "2026-03-25", account.ID)
	if err != nil || again.Created != 0 {
		t.Fatalf("second sync = %+v, err = %v", again, err)
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

	if _, err := svc.SetLoanTerms(ctx, domain.SetLoanTermsInput{
		AccountID: account.ID, AnchorDate: "2026-03-25", AnchorPrincipalCents: 90_000_000,
		AnnualRate: "5.00", Method: domain.LoanEqualInstallment, PaymentDay: 20, RemainingPeriods: 9,
	}); err != nil {
		t.Fatal(err)
	}
	if history, err := st.GetLoanHistory(ctx, account.ID); err != nil || history != nil {
		t.Fatalf("history should be removed when switching back: %+v, err = %v", history, err)
	}
}

func TestSyncHistoryWithoutAnchor(t *testing.T) {
	ctx := context.Background()
	_, st := newTestServiceAndStore(t)
	svc := domain.NewService(st, 0)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "测试房贷", Type: domain.AccountLiability})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetLoanHistory(ctx, historyInput(account.ID)); err != nil {
		t.Fatal(err)
	}
	result, err := svc.SyncLoans(ctx, "2026-03-25", account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 3 {
		t.Fatalf("sync = %+v", result)
	}
	snapshots, err := st.ListConfirmed(ctx, account.ID, "", "")
	if err != nil || len(snapshots) != 3 {
		t.Fatalf("snapshots = %d, err = %v", len(snapshots), err)
	}
	latest := snapshots[len(snapshots)-1]
	if latest.AsOf != "2026-03-20" || latest.Source != domain.SourceLoan ||
		latest.CashCents == nil || *latest.CashCents != -result.Accounts[0].BalanceCents {
		t.Fatalf("latest = %+v, entry = %+v", latest, result.Accounts[0])
	}
}
