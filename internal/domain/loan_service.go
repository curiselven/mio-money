package domain

import (
	"context"
	"fmt"
)

type SetLoanTermsInput struct {
	AccountID            string     `json:"account_id"`
	AnchorDate           string     `json:"anchor_date"`
	AnchorPrincipalCents int64      `json:"anchor_principal_cents"`
	AnnualRate           string     `json:"annual_rate"`
	Method               LoanMethod `json:"method"`
	PaymentDay           int        `json:"payment_day"`
	RemainingPeriods     int        `json:"remaining_periods"`
	// Origin records where the change came from ("mcp", "desktop"); blank is mcp.
	Origin string `json:"-"`
}

// SetLoanTerms stores terms for a liability account. When the anchor date has
// no snapshot yet, it also records the anchor balance as a confirmed snapshot.
func (s *Service) SetLoanTerms(ctx context.Context, in SetLoanTermsInput) (*LoanTerms, error) {
	account, err := s.repo.GetAccount(ctx, in.AccountID)
	if err != nil {
		return nil, err
	}
	if account.Type != AccountLiability {
		return nil, fmt.Errorf("account %s is %s; loan terms require a liability account", account.ID, account.Type)
	}
	now := s.now().UTC()
	terms := &LoanTerms{
		AccountID:            account.ID,
		AnchorDate:           in.AnchorDate,
		AnchorPrincipalCents: in.AnchorPrincipalCents,
		AnnualRate:           in.AnnualRate,
		Method:               in.Method,
		PaymentDay:           in.PaymentDay,
		RemainingPeriods:     in.RemainingPeriods,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := validateLoanTerms(terms); err != nil {
		return nil, err
	}
	existing, err := s.repo.GetLoan(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		terms.CreatedAt = existing.CreatedAt
	}
	if err := s.repo.UpsertLoan(ctx, terms); err != nil {
		return nil, err
	}
	origin := in.Origin
	if origin == "" {
		origin = "mcp"
	}
	if err := s.repo.AddAudit(ctx, AuditEntry{
		Action:    "loan_terms",
		AccountID: account.ID,
		Detail: fmt.Sprintf("来源 %s：%s，年利率 %s%%，%d 期，扣款日 %d",
			origin, terms.Method, terms.AnnualRate, terms.RemainingPeriods, terms.PaymentDay),
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}

	has, err := s.repo.HasSnapshotAt(ctx, account.ID, terms.AnchorDate)
	if err != nil {
		return nil, err
	}
	if !has {
		cash := -terms.AnchorPrincipalCents
		snapshot := &Snapshot{
			ID:          newID("snp"),
			AccountID:   account.ID,
			AsOf:        terms.AnchorDate,
			Status:      StatusConfirmed,
			Source:      SourceLoan,
			SourceNote:  "贷款起算本金",
			Currency:    account.Currency,
			CashCents:   &cash,
			Items:       []Item{},
			CreatedAt:   now,
			ConfirmedAt: &now,
		}
		if err := s.repo.InsertSnapshot(ctx, snapshot); err != nil {
			return nil, err
		}
		if err := s.audit(ctx, "loan_anchor", snapshot, fmt.Sprintf("起算本金 %d 分", terms.AnchorPrincipalCents)); err != nil {
			return nil, err
		}
	}
	return terms, nil
}

type LoanView struct {
	Terms                  LoanTerms    `json:"terms"`
	AsOf                   string       `json:"as_of"`
	BalanceCents           int64        `json:"balance_cents"`
	RemainingPeriods       int          `json:"remaining_periods"`
	Next                   *LoanPeriod  `json:"next_payment,omitempty"`
	RemainingInterestCents int64        `json:"remaining_interest_cents"`
	Schedule               []LoanPeriod `json:"schedule,omitempty"`
}

// Loan returns the terms plus the computed state at today's date.
func (s *Service) Loan(ctx context.Context, accountID string, includeSchedule bool) (*LoanView, error) {
	terms, err := s.repo.GetLoan(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if terms == nil {
		return nil, fmt.Errorf("account %s has no loan terms", accountID)
	}
	calc, err := LoanAt(*terms, s.today())
	if err != nil {
		return nil, err
	}
	view := &LoanView{
		Terms:                  *terms,
		AsOf:                   calc.AsOf,
		BalanceCents:           calc.BalanceCents,
		RemainingPeriods:       calc.RemainingPeriods,
		Next:                   calc.Next,
		RemainingInterestCents: calc.RemainingInterestCents,
	}
	if includeSchedule {
		view.Schedule, err = LoanSchedule(*terms)
		if err != nil {
			return nil, err
		}
	}
	return view, nil
}

type LoanBriefing struct {
	BalanceCents           int64     `json:"balance_cents"`
	RemainingPeriods       int       `json:"remaining_periods"`
	NextDate               string    `json:"next_date,omitempty"`
	NextPaymentCents       int64     `json:"next_payment_cents,omitempty"`
	RemainingInterestCents int64     `json:"remaining_interest_cents"`
	Terms                  LoanTerms `json:"terms"`
}

// LoanBriefings returns the desktop one-liner data keyed by account id.
func (s *Service) LoanBriefings(ctx context.Context) (map[string]LoanBriefing, error) {
	loans, err := s.repo.ListLoans(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]LoanBriefing{}
	for _, terms := range loans {
		calc, err := LoanAt(terms, s.today())
		if err != nil {
			return nil, err
		}
		briefing := LoanBriefing{
			BalanceCents:           calc.BalanceCents,
			RemainingPeriods:       calc.RemainingPeriods,
			RemainingInterestCents: calc.RemainingInterestCents,
			Terms:                  terms,
		}
		if calc.Next != nil {
			briefing.NextDate = calc.Next.Date
			briefing.NextPaymentCents = calc.Next.PaymentCents
		}
		out[terms.AccountID] = briefing
	}
	return out, nil
}

type LoanSyncAccount struct {
	AccountID        string `json:"account_id"`
	Name             string `json:"name"`
	CreatedSnapshots int    `json:"created_snapshots"`
	BalanceCents     int64  `json:"balance_cents"`
	RemainingPeriods int    `json:"remaining_periods"`
	NextDate         string `json:"next_date,omitempty"`
	NextPaymentCents int64  `json:"next_payment_cents,omitempty"`
}

type LoanSyncResult struct {
	AsOf     string            `json:"as_of"`
	Created  int               `json:"created_snapshots"`
	Accounts []LoanSyncAccount `json:"accounts"`
}

// SyncLoans writes one confirmed snapshot per payment date elapsed since the
// last confirmed snapshot (or the anchor date), up to today, for every loan or
// just accountID when it is non-empty. Re-running is a no-op because an
// existing snapshot on the same as_of is skipped.
func (s *Service) SyncLoans(ctx context.Context, today, accountID string) (*LoanSyncResult, error) {
	if err := validDate(today); err != nil {
		return nil, fmt.Errorf("today: %w", err)
	}
	loans, err := s.repo.ListLoans(ctx)
	if err != nil {
		return nil, err
	}
	result := &LoanSyncResult{AsOf: today, Accounts: []LoanSyncAccount{}}
	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	currencies := map[string]string{}
	for _, a := range accounts {
		names[a.ID] = a.Name
		currencies[a.ID] = a.Currency
	}
	for _, terms := range loans {
		if accountID != "" && terms.AccountID != accountID {
			continue
		}
		periods, err := LoanSchedule(terms)
		if err != nil {
			return nil, fmt.Errorf("account %s: %w", terms.AccountID, err)
		}
		after := terms.AnchorDate
		last, err := s.repo.LastConfirmedBefore(ctx, terms.AccountID, "")
		if err != nil {
			return nil, err
		}
		if last != nil && last.AsOf > after {
			after = last.AsOf
		}
		entry := LoanSyncAccount{AccountID: terms.AccountID, Name: names[terms.AccountID]}
		for _, p := range periods {
			if p.Date <= after || p.Date > today {
				continue
			}
			has, err := s.repo.HasSnapshotAt(ctx, terms.AccountID, p.Date)
			if err != nil {
				return nil, err
			}
			if has {
				continue
			}
			now := s.now().UTC()
			cash := -p.BalanceCents
			snapshot := &Snapshot{
				ID:          newID("snp"),
				AccountID:   terms.AccountID,
				AsOf:        p.Date,
				Status:      StatusConfirmed,
				Source:      SourceLoan,
				SourceNote:  fmt.Sprintf("第 %d 期还款后", p.Number),
				Currency:    currencies[terms.AccountID],
				CashCents:   &cash,
				Items:       []Item{},
				CreatedAt:   now,
				ConfirmedAt: &now,
			}
			if err := s.repo.InsertSnapshot(ctx, snapshot); err != nil {
				return nil, err
			}
			detail := fmt.Sprintf("第 %d 期后剩余本金 %d 分", p.Number, p.BalanceCents)
			if err := s.audit(ctx, "loan_sync", snapshot, detail); err != nil {
				return nil, err
			}
			entry.CreatedSnapshots++
			result.Created++
		}
		calc, err := LoanAt(terms, today)
		if err != nil {
			return nil, err
		}
		entry.BalanceCents = calc.BalanceCents
		entry.RemainingPeriods = calc.RemainingPeriods
		if calc.Next != nil {
			entry.NextDate = calc.Next.Date
			entry.NextPaymentCents = calc.Next.PaymentCents
		}
		result.Accounts = append(result.Accounts, entry)
	}
	return result, nil
}

func (s *Service) today() string { return s.now().Format("2006-01-02") }
