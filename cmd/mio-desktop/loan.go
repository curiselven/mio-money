package main

import (
	"context"
	"time"

	"github.com/curiselven/mio-money/internal/domain"
)

type LoanService struct {
	svc *domain.Service
}

func NewLoanService(svc *domain.Service) *LoanService {
	return &LoanService{svc: svc}
}

type LoanFormInput struct {
	AccountID            string `json:"account_id"`
	AnchorDate           string `json:"anchor_date"`
	AnchorPrincipalCents int64  `json:"anchor_principal_cents"`
	AnnualRate           string `json:"annual_rate"`
	Method               string `json:"method"`
	PaymentDay           int    `json:"payment_day"`
	RemainingPeriods     int    `json:"remaining_periods"`
}

func (in LoanFormInput) domainInput() domain.SetLoanTermsInput {
	return domain.SetLoanTermsInput{
		AccountID:            in.AccountID,
		AnchorDate:           in.AnchorDate,
		AnchorPrincipalCents: in.AnchorPrincipalCents,
		AnnualRate:           in.AnnualRate,
		Method:               domain.LoanMethod(in.Method),
		PaymentDay:           in.PaymentDay,
		RemainingPeriods:     in.RemainingPeriods,
	}
}

// Preview validates the form and returns the numbers without touching storage.
func (s *LoanService) Preview(in LoanFormInput) (*domain.LoanPreview, error) {
	return domain.PreviewLoan(domain.LoanTerms{
		AnchorDate:           in.AnchorDate,
		AnchorPrincipalCents: in.AnchorPrincipalCents,
		AnnualRate:           in.AnnualRate,
		Method:               domain.LoanMethod(in.Method),
		PaymentDay:           in.PaymentDay,
		RemainingPeriods:     in.RemainingPeriods,
	})
}

// Save uses the same domain path as MCP set_loan_terms, marks the audit entry
// as desktop, then syncs this account so the overview updates immediately.
func (s *LoanService) Save(in LoanFormInput) (*domain.LoanTerms, error) {
	ctx := context.Background()
	terms := in.domainInput()
	terms.Origin = "desktop"
	saved, err := s.svc.SetLoanTerms(ctx, terms)
	if err != nil {
		return nil, err
	}
	today := time.Now().Format("2006-01-02")
	if _, err := s.svc.SyncLoans(ctx, today, in.AccountID); err != nil {
		return nil, err
	}
	return saved, nil
}
