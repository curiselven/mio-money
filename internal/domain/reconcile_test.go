package domain_test

import (
	"testing"

	"github.com/curiselven/mio-money/internal/domain"
)

func reconcileHistory() domain.LoanHistory {
	h := testHistory()
	h.TotalPeriods = 24
	return h
}

func bankSamples(t *testing.T, h domain.LoanHistory, periods ...int) []domain.BankPeriodSample {
	t.Helper()
	schedule, err := domain.LoanHistorySchedule(h)
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]domain.BankPeriodSample, 0, len(periods))
	for _, number := range periods {
		period := schedule[number-1]
		balance := period.BalanceCents
		samples = append(samples, domain.BankPeriodSample{
			Date:           period.Date,
			PaymentCents:   period.PaymentCents,
			PrincipalCents: period.PrincipalCents,
			InterestCents:  period.InterestCents,
			BalanceCents:   &balance,
		})
	}
	return samples
}

func TestReconcileFindsDateAndRule(t *testing.T) {
	truth := reconcileHistory()
	truth.RateChanges = []domain.LoanRateChange{
		{Date: "2026-05-15", AnnualRate: "5.20", PrincipalRule: domain.PrincipalOldSchedule},
	}
	samples := bankSamples(t, truth, 5)

	wrong := reconcileHistory()
	wrong.RateChanges = []domain.LoanRateChange{
		{Date: "2026-05-18", AnnualRate: "5.20", PrincipalRule: domain.PrincipalNewSchedule},
	}
	result := domain.ReconcileLoanHistory(wrong, samples)
	if len(result.Samples) != 1 || !result.Samples[0].Matched || result.Matched != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Recommendations) != 1 {
		t.Fatalf("recommendations = %+v", result.Recommendations)
	}
	rec := result.Recommendations[0]
	if rec.ChangeDate != "2026-05-18" || rec.SuggestedDate != "2026-05-15" || rec.PrincipalRule != domain.PrincipalOldSchedule {
		t.Fatalf("recommendation = %+v", rec)
	}
	applied := result.AppliedHistory.RateChanges[0]
	if applied.Date != "2026-05-15" || applied.PrincipalRule != domain.PrincipalOldSchedule {
		t.Fatalf("applied = %+v", applied)
	}
	if result.Samples[0].BalanceDiffCents == nil || *result.Samples[0].BalanceDiffCents != 0 {
		t.Fatalf("balance diff = %v", result.Samples[0].BalanceDiffCents)
	}
}

func TestReconcileChainedAdjustments(t *testing.T) {
	truth := reconcileHistory()
	truth.RateChanges = []domain.LoanRateChange{
		{Date: "2026-04-10", AnnualRate: "5.00", PrincipalRule: domain.PrincipalFixedPayment},
		{Date: "2026-09-05", AnnualRate: "5.30", PrincipalRule: domain.PrincipalOldSchedule},
	}
	samples := bankSamples(t, truth, 4, 6, 9)

	wrong := reconcileHistory()
	wrong.RateChanges = []domain.LoanRateChange{
		{Date: "2026-04-12", AnnualRate: "5.00", PrincipalRule: domain.PrincipalNewSchedule},
		{Date: "2026-09-07", AnnualRate: "5.30", PrincipalRule: domain.PrincipalFixedPayment},
	}
	result := domain.ReconcileLoanHistory(wrong, samples)
	if result.Matched != 3 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Recommendations) != 2 {
		t.Fatalf("recommendations = %+v", result.Recommendations)
	}
	applied := result.AppliedHistory.RateChanges
	if applied[0].Date != "2026-04-10" || applied[0].PrincipalRule != domain.PrincipalFixedPayment ||
		applied[1].Date != "2026-09-05" || applied[1].PrincipalRule != domain.PrincipalOldSchedule {
		t.Fatalf("applied = %+v", applied)
	}
	for i, sample := range result.Samples {
		if !sample.Matched || sample.BalanceDiffCents == nil || *sample.BalanceDiffCents != 0 {
			t.Fatalf("sample %d = %+v", i, sample)
		}
	}
}

func TestReconcileNoSolutionReportsDiffs(t *testing.T) {
	h := reconcileHistory()
	h.RateChanges = []domain.LoanRateChange{
		{Date: "2026-05-15", AnnualRate: "5.20", PrincipalRule: domain.PrincipalNewSchedule},
	}
	samples := []domain.BankPeriodSample{{
		Date:           "2026-05-20",
		PaymentCents:   1,
		PrincipalCents: 1,
		InterestCents:  1,
	}}
	result := domain.ReconcileLoanHistory(h, samples)
	if result.Matched != 0 || len(result.Samples) != 1 || result.Samples[0].Matched {
		t.Fatalf("result = %+v", result)
	}
	sample := result.Samples[0]
	if sample.PaymentDiffCents == 0 && sample.PrincipalDiffCents == 0 && sample.InterestDiffCents == 0 {
		t.Fatalf("expected non-zero diffs: %+v", sample)
	}
}

func TestReconcilePrefersNoDateChangeAndDefaultRule(t *testing.T) {
	h := reconcileHistory()
	h.RateChanges = []domain.LoanRateChange{
		{Date: "2026-06-15", AnnualRate: "4.90", PrincipalRule: domain.PrincipalFixedPayment},
	}
	samples := bankSamples(t, h, 6)
	result := domain.ReconcileLoanHistory(h, samples)
	if len(result.Samples) != 1 || !result.Samples[0].Matched {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Recommendations) != 1 {
		t.Fatalf("recommendations = %+v", result.Recommendations)
	}
	rec := result.Recommendations[0]
	if rec.ChangeDate != "2026-06-15" || rec.SuggestedDate != "2026-06-15" || rec.PrincipalRule != domain.PrincipalNewSchedule {
		t.Fatalf("recommendation = %+v", rec)
	}
	if applied := result.AppliedHistory.RateChanges[0]; applied.PrincipalRule != domain.PrincipalNewSchedule {
		t.Fatalf("applied = %+v", applied)
	}
}

func TestReconcileNonAdjustmentSample(t *testing.T) {
	truth := reconcileHistory()
	truth.RateChanges = []domain.LoanRateChange{
		{Date: "2026-05-15", AnnualRate: "5.20", PrincipalRule: domain.PrincipalNewSchedule},
	}
	samples := bankSamples(t, truth, 8)
	samples[0].InterestCents++
	result := domain.ReconcileLoanHistory(truth, samples)
	if result.Matched != 0 || len(result.Recommendations) != 0 {
		t.Fatalf("result = %+v", result)
	}
	if result.Samples[0].InterestDiffCents != -1 {
		t.Fatalf("sample = %+v", result.Samples[0])
	}
}
