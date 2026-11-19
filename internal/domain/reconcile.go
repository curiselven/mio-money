package domain

import (
	"sort"
	"time"
)

type BankPeriodSample struct {
	Date           string `json:"date"`
	PaymentCents   int64  `json:"payment_cents"`
	PrincipalCents int64  `json:"principal_cents"`
	InterestCents  int64  `json:"interest_cents"`
	BalanceCents   *int64 `json:"balance_cents,omitempty"`
}

type ReconcileSampleResult struct {
	Date               string        `json:"date"`
	Matched            bool          `json:"matched"`
	Period             int           `json:"period,omitempty"`
	PaymentDiffCents   int64         `json:"payment_diff_cents"`
	PrincipalDiffCents int64         `json:"principal_diff_cents"`
	InterestDiffCents  int64         `json:"interest_diff_cents"`
	BalanceDiffCents   *int64        `json:"balance_diff_cents,omitempty"`
	PrincipalRule      PrincipalRule `json:"principal_rule,omitempty"`
}

type ReconcileRecommendation struct {
	ChangeDate    string        `json:"change_date"`
	SuggestedDate string        `json:"suggested_date"`
	PrincipalRule PrincipalRule `json:"principal_rule"`
}

type ReconcileResult struct {
	Samples         []ReconcileSampleResult   `json:"samples"`
	Matched         int                       `json:"matched_count"`
	Recommendations []ReconcileRecommendation `json:"recommendations"`
	AppliedHistory  LoanHistory               `json:"applied_history"`
}

type reconcileCandidate struct {
	changeIndex  int
	date         string
	rule         PrincipalRule
	originalDate string
	dateChanged  bool
	matched      bool
	score        int64
	dateDelta    int
}

func cloneLoanHistory(h LoanHistory) LoanHistory {
	out := h
	out.RateChanges = append([]LoanRateChange(nil), h.RateChanges...)
	out.Prepayments = append([]LoanPrepayment(nil), h.Prepayments...)
	return out
}

func findPeriod(periods []LoanPeriod, date string) int {
	for i, period := range periods {
		if period.Date == date {
			return i
		}
	}
	return -1
}

func periodPrevDate(periods []LoanPeriod, index int, h LoanHistory) string {
	if index > 0 {
		return periods[index-1].Date
	}
	if h.DisbursementDate != "" {
		return h.DisbursementDate
	}
	cur, err := time.Parse("2006-01-02", periods[index].Date)
	if err != nil {
		return ""
	}
	return cur.AddDate(0, 0, -30).Format("2006-01-02")
}

func reconciliationScore(period LoanPeriod, sample BankPeriodSample) int64 {
	interest := absInt64(period.InterestCents - sample.InterestCents)
	principal := absInt64(period.PrincipalCents - sample.PrincipalCents)
	return interest + principal
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func candidateBetter(c, best reconcileCandidate) bool {
	if c.matched != best.matched {
		return c.matched
	}
	if !c.matched && c.score != best.score {
		return c.score < best.score
	}
	if c.dateChanged != best.dateChanged {
		return !c.dateChanged
	}
	if (c.rule == PrincipalNewSchedule) != (best.rule == PrincipalNewSchedule) {
		return c.rule == PrincipalNewSchedule
	}
	if c.dateDelta != best.dateDelta {
		return c.dateDelta < best.dateDelta
	}
	return ruleRank(c.rule) < ruleRank(best.rule)
}

func ruleRank(rule PrincipalRule) int {
	switch rule {
	case PrincipalNewSchedule:
		return 0
	case PrincipalFixedPayment:
		return 1
	default:
		return 2
	}
}

func dateDeltaDays(from, to string) int {
	a, errA := time.Parse("2006-01-02", from)
	b, errB := time.Parse("2006-01-02", to)
	if errA != nil || errB != nil {
		return 0
	}
	days := int(b.Sub(a).Hours() / 24)
	if days < 0 {
		return -days
	}
	return days
}

// ReconcileLoanHistory guesses the rate-change dates and principal rules from
// bank samples. Samples are processed in date order; a correction changes the
// plan used for later samples. Diff fields are Mio - bank. Date changes are
// searched within ±10 days and clamped to the sample's period, and are only
// accepted when the resulting plan is valid.
func ReconcileLoanHistory(h LoanHistory, samples []BankPeriodSample) ReconcileResult {
	work := cloneLoanHistory(h)
	ordered := append([]BankPeriodSample(nil), samples...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Date < ordered[j].Date })
	result := ReconcileResult{
		Samples:         make([]ReconcileSampleResult, 0, len(ordered)),
		Recommendations: []ReconcileRecommendation{},
	}
	recommended := map[int]bool{}

	for _, sample := range ordered {
		out := ReconcileSampleResult{Date: sample.Date}
		periods, err := LoanHistorySchedule(work)
		if err != nil {
			result.Samples = append(result.Samples, out)
			continue
		}
		index := findPeriod(periods, sample.Date)
		if index < 0 {
			result.Samples = append(result.Samples, out)
			continue
		}
		out.Period = periods[index].Number
		prev := periodPrevDate(periods, index, work)

		var best *reconcileCandidate
		for changeIndex, change := range work.RateChanges {
			if change.Date <= prev || change.Date > sample.Date {
				continue
			}
			rate := change.AnnualRate
			for delta := -10; delta <= 10; delta++ {
				date := shiftDate(change.Date, delta)
				if date <= prev || date > sample.Date {
					continue
				}
				for _, rule := range []PrincipalRule{PrincipalNewSchedule, PrincipalFixedPayment, PrincipalOldSchedule} {
					trial := cloneLoanHistory(work)
					trial.RateChanges[changeIndex] = LoanRateChange{Date: date, AnnualRate: rate, PrincipalRule: rule}
					trialPeriods, err := LoanHistorySchedule(trial)
					if err != nil {
						continue
					}
					trialIndex := findPeriod(trialPeriods, sample.Date)
					if trialIndex < 0 {
						continue
					}
					period := trialPeriods[trialIndex]
					candidate := reconcileCandidate{
						changeIndex:  changeIndex,
						date:         date,
						rule:         rule,
						originalDate: change.Date,
						dateChanged:  date != change.Date,
						matched:      period.InterestCents == sample.InterestCents && period.PrincipalCents == sample.PrincipalCents,
						score:        reconciliationScore(period, sample),
						dateDelta:    dateDeltaDays(change.Date, date),
					}
					if best == nil || candidateBetter(candidate, *best) {
						chosen := candidate
						best = &chosen
					}
				}
			}
		}

		if best == nil {
			out.InterestDiffCents = periods[index].InterestCents - sample.InterestCents
			out.PrincipalDiffCents = periods[index].PrincipalCents - sample.PrincipalCents
			out.PaymentDiffCents = periods[index].PaymentCents - sample.PaymentCents
			out.Matched = out.InterestDiffCents == 0 && out.PrincipalDiffCents == 0
			if out.Matched {
				result.Matched++
			}
			result.Samples = append(result.Samples, out)
			continue
		}
		original := work.RateChanges[best.changeIndex]
		if best.dateChanged || best.rule != original.PrincipalRule.orDefault() {
			work.RateChanges[best.changeIndex] = LoanRateChange{
				Date:          best.date,
				AnnualRate:    original.AnnualRate,
				PrincipalRule: best.rule,
			}
			if !recommended[best.changeIndex] {
				recommended[best.changeIndex] = true
				result.Recommendations = append(result.Recommendations, ReconcileRecommendation{
					ChangeDate:    best.originalDate,
					SuggestedDate: best.date,
					PrincipalRule: best.rule,
				})
			}
		}
		if periods, err = LoanHistorySchedule(work); err == nil {
			if applied := findPeriod(periods, sample.Date); applied >= 0 {
				period := periods[applied]
				out.InterestDiffCents = period.InterestCents - sample.InterestCents
				out.PrincipalDiffCents = period.PrincipalCents - sample.PrincipalCents
				out.PaymentDiffCents = period.PaymentCents - sample.PaymentCents
				out.Matched = period.InterestCents == sample.InterestCents && period.PrincipalCents == sample.PrincipalCents
			}
		}
		out.PrincipalRule = best.rule
		if out.Matched {
			result.Matched++
		}
		result.Samples = append(result.Samples, out)
	}

	if periods, err := LoanHistorySchedule(work); err == nil {
		for i, sample := range ordered {
			if sample.BalanceCents == nil {
				continue
			}
			if index := findPeriod(periods, sample.Date); index >= 0 {
				diff := periods[index].BalanceCents - *sample.BalanceCents
				result.Samples[i].BalanceDiffCents = &diff
			}
		}
	}
	result.AppliedHistory = work
	return result
}

func shiftDate(date string, days int) string {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return parsed.AddDate(0, 0, days).Format("2006-01-02")
}
