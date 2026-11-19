package domain_test

import (
	"context"
	"testing"

	"github.com/curiselven/mio-money/internal/domain"
)

func testLoanTerms() domain.LoanTerms {
	return domain.LoanTerms{
		AccountID:            "acc_loan",
		AnchorDate:           "2026-01-01",
		AnchorPrincipalCents: 100_000_000,
		AnnualRate:           "3.10",
		Method:               domain.LoanEqualInstallment,
		PaymentDay:           20,
		RemainingPeriods:     240,
	}
}

func TestEqualInstallmentSchedule(t *testing.T) {
	terms := testLoanTerms()
	periods, err := domain.LoanSchedule(terms)
	if err != nil {
		t.Fatal(err)
	}
	if len(periods) != 240 {
		t.Fatalf("periods = %d", len(periods))
	}
	first := periods[0]
	if first.Number != 1 || first.Date != "2026-01-20" {
		t.Fatalf("first = %+v", first)
	}
	if first.PaymentCents != 559_617 || first.PrincipalCents != 301_284 || first.InterestCents != 258_333 {
		t.Fatalf("first = %+v", first)
	}
	if first.BalanceCents != 99_698_716 {
		t.Fatalf("first balance = %d", first.BalanceCents)
	}

	var principal int64
	balance := terms.AnchorPrincipalCents
	for i, p := range periods {
		if p.PaymentCents != p.PrincipalCents+p.InterestCents {
			t.Fatalf("period %d payment mismatch: %+v", i+1, p)
		}
		if p.PrincipalCents <= 0 || p.BalanceCents < 0 {
			t.Fatalf("period %d principal/balance: %+v", i+1, p)
		}
		if p.BalanceCents != balance-p.PrincipalCents {
			t.Fatalf("period %d balance: %+v", i+1, p)
		}
		principal += p.PrincipalCents
		balance = p.BalanceCents
	}
	if principal != terms.AnchorPrincipalCents {
		t.Fatalf("principal sum = %d", principal)
	}
	last := periods[len(periods)-1]
	if last.Date != "2045-12-20" || last.BalanceCents != 0 {
		t.Fatalf("last = %+v", last)
	}
	if last.PaymentCents != last.PrincipalCents+last.InterestCents {
		t.Fatalf("last = %+v", last)
	}
	// The last payment clears the balance, so it usually differs from the rest.
	if last.PaymentCents == first.PaymentCents {
		t.Fatalf("last payment should absorb the rounding remainder: %+v", last)
	}
}

func TestEqualPrincipalSchedule(t *testing.T) {
	terms := testLoanTerms()
	terms.Method = domain.LoanEqualPrincipal
	periods, err := domain.LoanSchedule(terms)
	if err != nil {
		t.Fatal(err)
	}
	first := periods[0]
	if first.PrincipalCents != 416_666 || first.InterestCents != 258_333 {
		t.Fatalf("first = %+v", first)
	}
	last := periods[len(periods)-1]
	if last.PrincipalCents != 416_826 || last.BalanceCents != 0 {
		t.Fatalf("last = %+v", last)
	}
	var sum int64
	for _, p := range periods {
		sum += p.PrincipalCents
		if p.InterestCents >= first.InterestCents && p.Number != 1 {
			t.Fatalf("interest should fall every period: %+v", p)
		}
	}
	if sum != terms.AnchorPrincipalCents {
		t.Fatalf("principal sum = %d", sum)
	}
}

func TestLoanAt(t *testing.T) {
	terms := testLoanTerms()

	atAnchor, err := domain.LoanAt(terms, "2026-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if atAnchor.BalanceCents != 100_000_000 || atAnchor.RemainingPeriods != 240 {
		t.Fatalf("at anchor = %+v", atAnchor)
	}
	if atAnchor.Next == nil || atAnchor.Next.Date != "2026-01-20" || atAnchor.Next.PaymentCents != 559_617 {
		t.Fatalf("next = %+v", atAnchor.Next)
	}

	afterFirst, err := domain.LoanAt(terms, "2026-01-20")
	if err != nil {
		t.Fatal(err)
	}
	if afterFirst.BalanceCents != 99_698_716 || afterFirst.RemainingPeriods != 239 {
		t.Fatalf("after first = %+v", afterFirst)
	}
	if afterFirst.Next == nil || afterFirst.Next.Date != "2026-02-20" {
		t.Fatalf("next = %+v", afterFirst.Next)
	}
	if afterFirst.RemainingInterestCents >= atAnchor.RemainingInterestCents {
		t.Fatalf("remaining interest should fall: %d -> %d", atAnchor.RemainingInterestCents, afterFirst.RemainingInterestCents)
	}

	paidOff, err := domain.LoanAt(terms, "2046-12-20")
	if err != nil {
		t.Fatal(err)
	}
	if paidOff.BalanceCents != 0 || paidOff.RemainingPeriods != 0 || paidOff.Next != nil || paidOff.RemainingInterestCents != 0 {
		t.Fatalf("paid off = %+v", paidOff)
	}
}

func TestLoanZeroRateClearsLastPeriod(t *testing.T) {
	terms := testLoanTerms()
	terms.AnnualRate = "0"
	periods, err := domain.LoanSchedule(terms)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range periods {
		if p.InterestCents != 0 {
			t.Fatalf("interest at 0%%: %+v", p)
		}
	}
	if last := periods[len(periods)-1]; last.BalanceCents != 0 {
		t.Fatalf("last = %+v", last)
	}
}

func TestPaymentDayOnAnchorMovesToNextMonth(t *testing.T) {
	terms := testLoanTerms()
	terms.AnchorDate = "2026-01-20"
	periods, err := domain.LoanSchedule(terms)
	if err != nil {
		t.Fatal(err)
	}
	if periods[0].Date != "2026-02-20" {
		t.Fatalf("first date = %s", periods[0].Date)
	}
}

func TestLoanTermValidation(t *testing.T) {
	cases := []struct {
		name   string
		change func(*domain.LoanTerms)
	}{
		{"rate too high", func(t *domain.LoanTerms) { t.AnnualRate = "30.01" }},
		{"rate not a decimal", func(t *domain.LoanTerms) { t.AnnualRate = "3,1" }},
		{"rate negative", func(t *domain.LoanTerms) { t.AnnualRate = "-1" }},
		{"periods zero", func(t *domain.LoanTerms) { t.RemainingPeriods = 0 }},
		{"payment day zero", func(t *domain.LoanTerms) { t.PaymentDay = 0 }},
		{"payment day 29", func(t *domain.LoanTerms) { t.PaymentDay = 29 }},
		{"principal zero", func(t *domain.LoanTerms) { t.AnchorPrincipalCents = 0 }},
		{"bad date", func(t *domain.LoanTerms) { t.AnchorDate = "2026-13-01" }},
		{"bad method", func(t *domain.LoanTerms) { t.Method = "interest_only" }},
	}
	for _, tc := range cases {
		terms := testLoanTerms()
		tc.change(&terms)
		if _, err := domain.LoanSchedule(terms); err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
	}
}

func TestSetLoanTermsAndSync(t *testing.T) {
	ctx := context.Background()
	_, st := newTestServiceAndStore(t)
	svc := domain.NewService(st, 0)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "测试房贷", Type: domain.AccountLiability})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetLoanTerms(ctx, domain.SetLoanTermsInput{
		AccountID: account.ID, AnchorDate: "2026-01-01", AnchorPrincipalCents: 100_000_000,
		AnnualRate: "3.10", Method: domain.LoanEqualInstallment, PaymentDay: 20, RemainingPeriods: 240,
	}); err != nil {
		t.Fatal(err)
	}
	anchor, err := st.LastConfirmedBefore(ctx, account.ID, "2026-01-01")
	if err != nil || anchor == nil {
		t.Fatalf("anchor snapshot = %+v, err = %v", anchor, err)
	}
	if anchor.Source != domain.SourceLoan || anchor.CashCents == nil || *anchor.CashCents != -100_000_000 {
		t.Fatalf("anchor = %+v", anchor)
	}

	// Re-setting terms keeps a single anchor snapshot.
	if _, err := svc.SetLoanTerms(ctx, domain.SetLoanTermsInput{
		AccountID: account.ID, AnchorDate: "2026-01-01", AnchorPrincipalCents: 100_000_000,
		AnnualRate: "3.10", Method: domain.LoanEqualInstallment, PaymentDay: 20, RemainingPeriods: 240,
	}); err != nil {
		t.Fatal(err)
	}
	confirmed, err := st.ListConfirmed(ctx, account.ID, "", "")
	if err != nil || len(confirmed) != 1 {
		t.Fatalf("confirmed = %d, err = %v", len(confirmed), err)
	}

	// Sync to just after the third payment date. Threshold is 0, which must not
	// block loan snapshots.
	result, err := svc.SyncLoans(ctx, "2026-03-25")
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 3 {
		t.Fatalf("sync = %+v", result)
	}
	entry := result.Accounts[0]
	if entry.AccountID != account.ID || entry.CreatedSnapshots != 3 || entry.RemainingPeriods != 237 {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.NextDate != "2026-04-20" || entry.NextPaymentCents != 559_617 {
		t.Fatalf("entry = %+v", entry)
	}
	latest, err := st.LastConfirmedBefore(ctx, account.ID, "")
	if err != nil || latest == nil {
		t.Fatalf("latest = %+v, err = %v", latest, err)
	}
	if latest.AsOf != "2026-03-20" || latest.Source != domain.SourceLoan || latest.CashCents == nil || *latest.CashCents != -entry.BalanceCents {
		t.Fatalf("latest = %+v", latest)
	}
	audit, err := st.ListAudit(ctx, latest.ID)
	if err != nil || len(audit) == 0 || audit[0].Action != "loan_sync" {
		t.Fatalf("audit = %+v, err = %v", audit, err)
	}

	again, err := svc.SyncLoans(ctx, "2026-03-25")
	if err != nil {
		t.Fatal(err)
	}
	if again.Created != 0 {
		t.Fatalf("second sync = %+v", again)
	}
	all, err := st.ListConfirmed(ctx, account.ID, "", "")
	if err != nil || len(all) != 4 {
		t.Fatalf("confirmed = %d, err = %v", len(all), err)
	}
}

func TestSetLoanTermsRejectsNonLiability(t *testing.T) {
	ctx := context.Background()
	_, st := newTestServiceAndStore(t)
	svc := domain.NewService(st, 0.2)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "测试基金", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SetLoanTerms(ctx, domain.SetLoanTermsInput{
		AccountID: account.ID, AnchorDate: "2026-01-01", AnchorPrincipalCents: 1_000,
		AnnualRate: "3.10", Method: domain.LoanEqualInstallment, PaymentDay: 20, RemainingPeriods: 12,
	})
	if err == nil {
		t.Fatal("expected an error for a non-liability account")
	}
	if loan, err := st.GetLoan(ctx, account.ID); err != nil || loan != nil {
		t.Fatalf("loan = %+v, err = %v", loan, err)
	}
	if _, err := svc.Loan(ctx, account.ID, false); err == nil {
		t.Fatal("expected get_loan to fail without terms")
	}
}
