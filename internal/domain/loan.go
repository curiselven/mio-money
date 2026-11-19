package domain

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

type LoanMethod string

const (
	LoanEqualInstallment LoanMethod = "equal_installment"
	LoanEqualPrincipal   LoanMethod = "equal_principal"
)

func (m LoanMethod) Valid() bool {
	return m == LoanEqualInstallment || m == LoanEqualPrincipal
}

// LoanTerms describes one loan. anchor_date/anchor_principal_cents are the
// remaining principal at that date (the first day of this terms set), so
// resetting rate or prepaying means storing a new terms row over the old one.
type LoanTerms struct {
	AccountID            string     `json:"account_id"`
	AnchorDate           string     `json:"anchor_date"`
	AnchorPrincipalCents int64      `json:"anchor_principal_cents"`
	AnnualRate           string     `json:"annual_rate"`
	Method               LoanMethod `json:"method"`
	PaymentDay           int        `json:"payment_day"`
	RemainingPeriods     int        `json:"remaining_periods"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// LoanPeriod is one scheduled payment. BalanceCents is the balance after it.
type LoanPeriod struct {
	Number         int           `json:"period"`
	Date           string        `json:"date"`
	AnnualRate     string        `json:"annual_rate,omitempty"`
	RateSplit      bool          `json:"rate_split,omitempty"`
	PrincipalRule  PrincipalRule `json:"principal_rule,omitempty"`
	PaymentCents   int64         `json:"payment_cents"`
	PrincipalCents int64         `json:"principal_cents"`
	InterestCents  int64         `json:"interest_cents"`
	BalanceCents   int64         `json:"balance_cents"`
}

// LoanCalculation is the loan state at one date. Next is nil once paid off.
type LoanCalculation struct {
	AsOf                   string      `json:"as_of"`
	BalanceCents           int64       `json:"balance_cents"`
	PaidPeriods            int         `json:"paid_periods"`
	PaidPrincipalCents     int64       `json:"paid_principal_cents"`
	PaidInterestCents      int64       `json:"paid_interest_cents"`
	PrepaidCents           int64       `json:"prepaid_cents"`
	PaidTotalCents         int64       `json:"paid_total_cents"`
	RemainingPeriods       int         `json:"remaining_periods"`
	Next                   *LoanPeriod `json:"next_payment,omitempty"`
	RemainingInterestCents int64       `json:"remaining_interest_cents"`
}

// LoanPreview is the desktop form preview: no storage, just the schedule.
type LoanPreview struct {
	PaymentCents           int64  `json:"payment_cents"`
	LastPaymentCents       int64  `json:"last_payment_cents"`
	NextDate               string `json:"next_date"`
	LastDate               string `json:"last_date"`
	RemainingInterestCents int64  `json:"remaining_interest_cents"`
}

var loanRatePattern = regexp.MustCompile(`^\d+(\.\d+)?$`)

// loanMonthlyRate parses "3.10" (annual percent) into 31/12000.
func loanMonthlyRate(annual string) (*big.Rat, error) {
	v := strings.TrimSpace(annual)
	if !loanRatePattern.MatchString(v) {
		return nil, fmt.Errorf("annual_rate %q must be a plain decimal like \"3.10\"", annual)
	}
	rate, ok := new(big.Rat).SetString(v)
	if !ok {
		return nil, fmt.Errorf("annual_rate %q is not a number", annual)
	}
	if rate.Cmp(big.NewRat(30, 1)) > 0 {
		return nil, fmt.Errorf("annual_rate %s is out of range, must be 0-30 (percent)", v)
	}
	return rate.Quo(rate, big.NewRat(1200, 1)), nil
}

func validateLoanTerms(t *LoanTerms) error {
	if err := validDate(t.AnchorDate); err != nil {
		return fmt.Errorf("anchor_date: %w", err)
	}
	if t.AnchorPrincipalCents <= 0 {
		return errors.New("anchor_principal_cents must be > 0")
	}
	if _, err := loanMonthlyRate(t.AnnualRate); err != nil {
		return err
	}
	if !t.Method.Valid() {
		return fmt.Errorf("method %q must be equal_installment or equal_principal", t.Method)
	}
	if t.PaymentDay < 1 || t.PaymentDay > 28 {
		return errors.New("payment_day must be between 1 and 28")
	}
	if t.RemainingPeriods <= 0 {
		return errors.New("remaining_periods must be > 0")
	}
	return nil
}

// loanPaymentDates returns the payment dates strictly after anchor_date,
// stepping monthly on payment_day (<= 28, so no month-length edge cases).
func loanPaymentDates(anchor string, day, periods int) ([]string, error) {
	start, err := time.Parse("2006-01-02", anchor)
	if err != nil {
		return nil, fmt.Errorf("anchor_date: %w", err)
	}
	first := time.Date(start.Year(), start.Month(), day, 0, 0, 0, 0, time.UTC)
	if !first.After(start) {
		first = first.AddDate(0, 1, 0)
	}
	dates := make([]string, periods)
	for i := range dates {
		dates[i] = first.AddDate(0, i, 0).Format("2006-01-02")
	}
	return dates, nil
}

// roundRatCents rounds an exact cents value to whole cents, half away from zero.
func roundRatCents(r *big.Rat) int64 {
	num := new(big.Int).Abs(r.Num())
	den := r.Denom()
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if rem.Mul(rem, big.NewInt(2)).Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if r.Sign() < 0 {
		q.Neg(q)
	}
	return q.Int64()
}

func interestCents(balance int64, monthly *big.Rat) int64 {
	return roundRatCents(new(big.Rat).Mul(new(big.Rat).SetInt64(balance), monthly))
}

// installmentPayment is the standard equal-installment payment, rounded to cents.
func installmentPayment(principal int64, monthly *big.Rat, periods int) int64 {
	factor := new(big.Rat).Add(big.NewRat(1, 1), monthly)
	growth := new(big.Rat).SetInt64(1)
	for i := 0; i < periods; i++ {
		growth.Mul(growth, factor)
	}
	denom := new(big.Rat).Sub(growth, big.NewRat(1, 1))
	if denom.Sign() == 0 {
		return principal / int64(periods)
	}
	payment := new(big.Rat).SetInt64(principal)
	payment.Mul(payment, monthly)
	payment.Mul(payment, growth)
	return roundRatCents(payment.Quo(payment, denom))
}

// LoanSchedule returns every remaining payment from the anchor terms.
func LoanSchedule(t LoanTerms) ([]LoanPeriod, error) {
	if err := validateLoanTerms(&t); err != nil {
		return nil, err
	}
	monthly, err := loanMonthlyRate(t.AnnualRate)
	if err != nil {
		return nil, err
	}
	dates, err := loanPaymentDates(t.AnchorDate, t.PaymentDay, t.RemainingPeriods)
	if err != nil {
		return nil, err
	}
	n := t.RemainingPeriods
	periods := make([]LoanPeriod, 0, n)
	balance := t.AnchorPrincipalCents

	if t.Method == LoanEqualPrincipal {
		base := balance / int64(n)
		for i := 0; i < n; i++ {
			interest := interestCents(balance, monthly)
			principal := base
			if i == n-1 {
				principal = balance
			}
			balance -= principal
			periods = append(periods, LoanPeriod{
				Number:         i + 1,
				Date:           dates[i],
				AnnualRate:     t.AnnualRate,
				PaymentCents:   principal + interest,
				PrincipalCents: principal,
				InterestCents:  interest,
				BalanceCents:   balance,
			})
		}
		return periods, nil
	}

	payment := installmentPayment(t.AnchorPrincipalCents, monthly, n)
	for i := 0; i < n; i++ {
		interest := interestCents(balance, monthly)
		principal := payment - interest
		if principal < 1 {
			principal = 1
		}
		if i == n-1 || principal > balance {
			principal = balance
		}
		balance -= principal
		periods = append(periods, LoanPeriod{
			Number:         i + 1,
			Date:           dates[i],
			AnnualRate:     t.AnnualRate,
			PaymentCents:   principal + interest,
			PrincipalCents: principal,
			InterestCents:  interest,
			BalanceCents:   balance,
		})
	}
	return periods, nil
}

// LoanAt reports the balance and next payment at asOf. Payment dates on or
// before asOf count as already paid.
func LoanAt(t LoanTerms, asOf string) (*LoanCalculation, error) {
	if err := validDate(asOf); err != nil {
		return nil, fmt.Errorf("as_of: %w", err)
	}
	periods, err := LoanSchedule(t)
	if err != nil {
		return nil, err
	}
	return calculationAt(periods, asOf, t.AnchorPrincipalCents, nil), nil
}

// calculationAt treats payment dates on or before asOf as already paid.
func calculationAt(periods []LoanPeriod, asOf string, principal int64, prepayments []LoanPrepayment) *LoanCalculation {
	out := &LoanCalculation{AsOf: asOf, BalanceCents: principal, RemainingPeriods: len(periods)}
	paid := 0
	for _, p := range periods {
		if p.Date > asOf {
			out.RemainingInterestCents += p.InterestCents
			continue
		}
		paid++
		out.BalanceCents = p.BalanceCents
		out.PaidPrincipalCents += p.PrincipalCents
		out.PaidInterestCents += p.InterestCents
	}
	for _, prepayment := range prepayments {
		if prepayment.Date <= asOf {
			out.PrepaidCents += prepayment.AmountCents
		}
	}
	out.PaidTotalCents = out.PaidPrincipalCents + out.PaidInterestCents + out.PrepaidCents
	out.PaidPeriods = paid
	out.RemainingPeriods -= paid
	if paid < len(periods) {
		next := periods[paid]
		out.Next = &next
	}
	return out
}

// PreviewLoan validates the terms and returns the numbers shown in the form.
// PaymentCents is the equal-installment payment, or the first payment for
// equal principal (LastPaymentCents carries the final one).
func PreviewLoan(t LoanTerms) (*LoanPreview, error) {
	periods, err := LoanSchedule(t)
	if err != nil {
		return nil, err
	}
	var interest int64
	for _, p := range periods {
		interest += p.InterestCents
	}
	first, last := periods[0], periods[len(periods)-1]
	return &LoanPreview{
		PaymentCents:           first.PaymentCents,
		LastPaymentCents:       last.PaymentCents,
		NextDate:               first.Date,
		LastDate:               last.Date,
		RemainingInterestCents: interest,
	}, nil
}
