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

type LoanHistoryFormInput struct {
	AccountID              string                  `json:"account_id"`
	OriginalPrincipalCents int64                   `json:"original_principal_cents"`
	FirstPaymentDate       string                  `json:"first_payment_date"`
	TotalPeriods           int                     `json:"total_periods"`
	Method                 string                  `json:"method"`
	InitialAnnualRate      string                  `json:"initial_annual_rate"`
	RateChanges            []domain.LoanRateChange `json:"rate_changes"`
	Prepayments            []domain.LoanPrepayment `json:"prepayments"`
}

func (in LoanHistoryFormInput) history() domain.LoanHistory {
	return domain.LoanHistory{
		AccountID:              in.AccountID,
		OriginalPrincipalCents: in.OriginalPrincipalCents,
		FirstPaymentDate:       in.FirstPaymentDate,
		TotalPeriods:           in.TotalPeriods,
		Method:                 domain.LoanMethod(in.Method),
		InitialAnnualRate:      in.InitialAnnualRate,
		RateChanges:            in.RateChanges,
		Prepayments:            in.Prepayments,
	}
}

func (in LoanHistoryFormInput) domainInput(origin string) domain.SetLoanHistoryInput {
	history := in.history()
	return domain.SetLoanHistoryInput{
		AccountID:              history.AccountID,
		OriginalPrincipalCents: history.OriginalPrincipalCents,
		FirstPaymentDate:       history.FirstPaymentDate,
		TotalPeriods:           history.TotalPeriods,
		Method:                 history.Method,
		InitialAnnualRate:      history.InitialAnnualRate,
		RateChanges:            history.RateChanges,
		Prepayments:            history.Prepayments,
		Origin:                 origin,
	}
}

// PreviewHistory validates the full-history plan without storing it.
func (s *LoanService) PreviewHistory(in LoanHistoryFormInput) (*domain.LoanHistoryView, error) {
	return domain.LoanHistoryAt(in.history(), time.Now().Format("2006-01-02"))
}

// SaveHistory replaces the account's plan and syncs its snapshots.
func (s *LoanService) SaveHistory(in LoanHistoryFormInput) (*domain.LoanHistory, error) {
	ctx := context.Background()
	history, err := s.svc.SetLoanHistory(ctx, in.domainInput("desktop"))
	if err != nil {
		return nil, err
	}
	today := time.Now().Format("2006-01-02")
	if _, err := s.svc.SyncLoans(ctx, today, in.AccountID); err != nil {
		return nil, err
	}
	return history, nil
}
