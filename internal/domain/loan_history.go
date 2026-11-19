package domain

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"
)

type LoanRateChange struct {
	Date       string `json:"date"`
	AnnualRate string `json:"annual_rate"`
}

type PrepaymentMode string

const (
	PrepayShortenPeriods PrepaymentMode = "shorten_periods"
	PrepayReducePayment  PrepaymentMode = "reduce_payment"
)

func (m PrepaymentMode) Valid() bool {
	return m == PrepayShortenPeriods || m == PrepayReducePayment
}

type LoanPrepayment struct {
	Date        string         `json:"date"`
	AmountCents int64          `json:"amount_cents"`
	Mode        PrepaymentMode `json:"mode"`
}

// LoanHistory replays the loan from period 1, the way a bank does: the payment
// is recomputed with the remaining principal and periods at each rate change.
type LoanHistory struct {
	AccountID              string           `json:"account_id"`
	OriginalPrincipalCents int64            `json:"original_principal_cents"`
	DisbursementDate       string           `json:"disbursement_date,omitempty"`
	FirstPaymentDate       string           `json:"first_payment_date"`
	TotalPeriods           int              `json:"total_periods"`
	Method                 LoanMethod       `json:"method"`
	InitialAnnualRate      string           `json:"initial_annual_rate"`
	RateChanges            []LoanRateChange `json:"rate_changes"`
	Prepayments            []LoanPrepayment `json:"prepayments"`
	CreatedAt              time.Time        `json:"created_at"`
	UpdatedAt              time.Time        `json:"updated_at"`
}

// LoanHistoryView is the full history schedule plus the state at AsOf.
type LoanHistoryView struct {
	History LoanHistory `json:"history"`
	LoanCalculation
	Periods []LoanPeriod `json:"periods,omitempty"`
}

func validateLoanHistory(h *LoanHistory) error {
	if h.OriginalPrincipalCents <= 0 {
		return errors.New("original_principal_cents must be > 0")
	}
	if err := validDate(h.FirstPaymentDate); err != nil {
		return fmt.Errorf("first_payment_date: %w", err)
	}
	if h.DisbursementDate != "" {
		if err := validDate(h.DisbursementDate); err != nil {
			return fmt.Errorf("disbursement_date: %w", err)
		}
		if h.DisbursementDate >= h.FirstPaymentDate {
			return errors.New("disbursement_date must be before first_payment_date")
		}
	}
	if h.TotalPeriods <= 0 {
		return errors.New("total_periods must be > 0")
	}
	if h.TotalPeriods > 1200 {
		return errors.New("total_periods must be <= 1200")
	}
	if !h.Method.Valid() {
		return fmt.Errorf("method %q must be equal_installment or equal_principal", h.Method)
	}
	if _, err := loanMonthlyRate(h.InitialAnnualRate); err != nil {
		return fmt.Errorf("initial_annual_rate: %w", err)
	}
	seen := map[string]bool{}
	for i, change := range h.RateChanges {
		if err := validDate(change.Date); err != nil {
			return fmt.Errorf("rate_changes[%d].date: %w", i, err)
		}
		if _, err := loanMonthlyRate(change.AnnualRate); err != nil {
			return fmt.Errorf("rate_changes[%d].annual_rate: %w", i, err)
		}
		if seen[change.Date] {
			return fmt.Errorf("rate_changes[%d]: duplicate date %s", i, change.Date)
		}
		seen[change.Date] = true
	}
	lastDate, err := historyPaymentDate(h.FirstPaymentDate, h.TotalPeriods)
	if err != nil {
		return err
	}
	for i, prepay := range h.Prepayments {
		if err := validDate(prepay.Date); err != nil {
			return fmt.Errorf("prepayments[%d].date: %w", i, err)
		}
		if prepay.AmountCents <= 0 {
			return fmt.Errorf("prepayments[%d].amount_cents must be > 0", i)
		}
		if !prepay.Mode.Valid() {
			return fmt.Errorf("prepayments[%d].mode %q must be shorten_periods or reduce_payment", i, prepay.Mode)
		}
		if prepay.Date < h.FirstPaymentDate || prepay.Date >= lastDate {
			return fmt.Errorf("prepayments[%d].date must be between the first payment date and the last payment date", i)
		}
	}
	return nil
}

// historyPaymentDate returns the date of period n (1-based), preserving the
// first payment's day of month, clamped to the end of shorter months.
func historyPaymentDate(first string, n int) (string, error) {
	start, err := time.Parse("2006-01-02", first)
	if err != nil {
		return "", fmt.Errorf("first_payment_date: %w", err)
	}
	month := time.Date(start.Year(), start.Month()+time.Month(n-1), 1, 0, 0, 0, 0, time.UTC)
	day := start.Day()
	if last := month.AddDate(0, 1, -1).Day(); day > last {
		day = last
	}
	return time.Date(month.Year(), month.Month(), day, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), nil
}

func historyPaymentDates(first string, n int) ([]string, error) {
	dates := make([]string, n)
	for i := range dates {
		date, err := historyPaymentDate(first, i+1)
		if err != nil {
			return nil, err
		}
		dates[i] = date
	}
	return dates, nil
}

// historyRateAt returns the rate used to recompute the monthly payment for a
// period ending on date: a change applies from the first payment strictly after
// its effective date. Interest within the transition period is split by
// historyPeriodInterest; a change on the payment date has 0 new days there.
func historyRateAt(changes []LoanRateChange, initial, date string) string {
	rate := initial
	for _, change := range changes {
		if change.Date >= date {
			break
		}
		rate = change.AnnualRate
	}
	return rate
}

// historyFirstPeriodInterest uses the domestic actual/360 convention:
// principal * annual rate * actual days / 360. Expressed with the monthly rate
// that is monthly * days / 30; switch the 30 to 365/12 * 30 here for actual/365.
func historyFirstPeriodInterest(principal int64, monthly *big.Rat, days int) int64 {
	interest := new(big.Rat).SetInt64(principal)
	interest.Mul(interest, monthly)
	interest.Mul(interest, big.NewRat(int64(days), 30))
	return roundRatCents(interest)
}

// historyPeriodInterest applies the bank convention verified against a real
// repayment statement: each period is modeled as 30 days ending on curDate. A
// rate change effective on cd with curDate-30 < cd <= curDate splits the
// interest into actual-day segments. A split needs prevDate < cd < curDate (no
// prevDate means the first period): curDate-30 is the day-count anchor, so the
// first segment starts there and the last ends at curDate, summing to 30 days.
// All segments use their own annual rate. A change on the payment date itself
// gets 0 new days (old rate for the whole period); a change at or before
// curDate-30 means the whole period uses the new rate. Multiple changes are
// split in time order. The result is rounded to cents once, after summing exact
// rationals.
func historyPeriodInterest(balance int64, prevDate, curDate, initial string, changes []LoanRateChange, fallback *big.Rat) (int64, bool) {
	cur, err := time.Parse("2006-01-02", curDate)
	if err != nil {
		return interestCents(balance, fallback), false
	}
	start := cur.AddDate(0, 0, -30).Format("2006-01-02")
	points := []string{start}
	for _, change := range changes {
		if change.Date > start && (prevDate == "" || change.Date > prevDate) && change.Date < curDate {
			points = append(points, change.Date)
		}
	}
	if len(points) == 1 {
		return interestCents(balance, fallback), false
	}
	points = append(points, curDate)
	total := new(big.Rat)
	for i := 0; i+1 < len(points); i++ {
		days := daysBetween(points[i], points[i+1])
		if days <= 0 {
			continue
		}
		monthly, err := loanMonthlyRate(historyRateAt(changes, initial, points[i+1]))
		if err != nil {
			return interestCents(balance, fallback), false
		}
		part := new(big.Rat).SetInt64(balance)
		part.Mul(part, monthly)
		part.Mul(part, big.NewRat(int64(days), 30))
		total.Add(total, part)
	}
	return roundRatCents(total), true
}

func daysBetween(start, end string) int {
	from, _ := time.Parse("2006-01-02", start)
	to, _ := time.Parse("2006-01-02", end)
	return int(to.Sub(from).Hours() / 24)
}

// periodsToPayoff simulates cents-rounded payments to see how many periods the
// current payment still needs.
func periodsToPayoff(balance, payment int64, monthly *big.Rat) int {
	if balance <= 0 || payment <= 0 {
		return 0
	}
	for n := 1; n <= 1200; n++ {
		principal := payment - interestCents(balance, monthly)
		if principal <= 0 {
			return 1200
		}
		balance -= principal
		if balance <= 0 {
			return n
		}
	}
	return 1200
}

// LoanHistorySchedule replays every period from the original principal.
// Prepayments dated between payments are applied before the next payment;
// same-day prepayments are applied after that payment.
func LoanHistorySchedule(h LoanHistory) ([]LoanPeriod, error) {
	if err := validateLoanHistory(&h); err != nil {
		return nil, err
	}
	dates, err := historyPaymentDates(h.FirstPaymentDate, h.TotalPeriods)
	if err != nil {
		return nil, err
	}
	changes := append([]LoanRateChange(nil), h.RateChanges...)
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Date < changes[j].Date })
	prepays := append([]LoanPrepayment(nil), h.Prepayments...)
	sort.SliceStable(prepays, func(i, j int) bool { return prepays[i].Date < prepays[j].Date })

	balance := h.OriginalPrincipalCents
	total := h.TotalPeriods
	rate := h.InitialAnnualRate
	monthly, err := loanMonthlyRate(rate)
	if err != nil {
		return nil, err
	}
	payment := int64(0)
	base := int64(0)
	if h.Method == LoanEqualInstallment {
		payment = installmentPayment(balance, monthly, total)
	} else {
		base = balance / int64(total)
	}

	periods := make([]LoanPeriod, 0, total)
	pi := 0
	for k := 1; k <= total; k++ {
		date := dates[k-1]
		nextRate := historyRateAt(changes, h.InitialAnnualRate, date)
		if nextRate != rate {
			rate = nextRate
			monthly, err = loanMonthlyRate(rate)
			if err != nil {
				return nil, err
			}
			remaining := total - k + 1
			if h.Method == LoanEqualInstallment {
				payment = installmentPayment(balance, monthly, remaining)
			} else {
				base = balance / int64(remaining)
			}
		}

		applyPrepayment := func(prepay LoanPrepayment, afterPeriod int) {
			if prepay.AmountCents >= balance {
				balance = 0
			} else {
				balance -= prepay.AmountCents
			}
			if balance == 0 {
				total = afterPeriod
				return
			}
			remaining := total - afterPeriod
			if remaining <= 0 {
				return
			}
			switch prepay.Mode {
			case PrepayReducePayment:
				if h.Method == LoanEqualInstallment {
					payment = installmentPayment(balance, monthly, remaining)
				} else {
					base = balance / int64(remaining)
				}
			case PrepayShortenPeriods:
				var n int
				if h.Method == LoanEqualInstallment {
					n = periodsToPayoff(balance, payment, monthly)
				} else {
					n = int((balance + base - 1) / base)
				}
				if n > 0 && afterPeriod+n < total {
					total = afterPeriod + n
				}
			}
		}

		for pi < len(prepays) && prepays[pi].Date < date {
			applyPrepayment(prepays[pi], k-1)
			pi++
		}
		if balance == 0 {
			break
		}

		interest := interestCents(balance, monthly)
		rateSplit := false
		if !(k == 1 && h.DisbursementDate != "") {
			prevDate := ""
			if k > 1 {
				prevDate = dates[k-2]
			}
			interest, rateSplit = historyPeriodInterest(balance, prevDate, date, h.InitialAnnualRate, changes, monthly)
		}
		var principal int64
		if h.Method == LoanEqualInstallment {
			principal = payment - interest
			if principal < 1 {
				principal = 1
			}
		} else {
			principal = base
			if principal < 1 {
				principal = 1
			}
		}
		if k == total || principal > balance {
			principal = balance
		}
		if k == 1 && h.DisbursementDate != "" {
			interest = historyFirstPeriodInterest(balance, monthly, daysBetween(h.DisbursementDate, date))
		}
		balance -= principal
		periods = append(periods, LoanPeriod{
			Number:         k,
			Date:           date,
			AnnualRate:     rate,
			RateSplit:      rateSplit,
			PaymentCents:   principal + interest,
			PrincipalCents: principal,
			InterestCents:  interest,
			BalanceCents:   balance,
		})

		for pi < len(prepays) && prepays[pi].Date == date {
			applyPrepayment(prepays[pi], k)
			pi++
		}
		if balance == 0 {
			break
		}
	}
	return periods, nil
}

// LoanHistoryAt replays the schedule and reports the state at asOf.
func LoanHistoryAt(h LoanHistory, asOf string) (*LoanHistoryView, error) {
	if err := validDate(asOf); err != nil {
		return nil, fmt.Errorf("as_of: %w", err)
	}
	periods, err := LoanHistorySchedule(h)
	if err != nil {
		return nil, err
	}
	return &LoanHistoryView{
		History:         h,
		LoanCalculation: *calculationAt(periods, asOf, h.OriginalPrincipalCents, h.Prepayments),
		Periods:         periods,
	}, nil
}
