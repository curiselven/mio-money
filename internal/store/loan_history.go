package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/curiselven/mio-money/internal/domain"
)

const loanHistoryColumns = `account_id, original_principal_cents, first_payment_date, total_periods,
	method, initial_annual_rate, rate_changes, prepayments, created_at, updated_at`

// GetLoanHistory returns the account's full-history plan, or nil when absent.
func (s *Store) GetLoanHistory(ctx context.Context, accountID string) (*domain.LoanHistory, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+loanHistoryColumns+` FROM loan_histories WHERE account_id = ?`, accountID)
	history, err := scanLoanHistory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return history, err
}

func (s *Store) ListLoanHistories(ctx context.Context) ([]domain.LoanHistory, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+loanHistoryColumns+` FROM loan_histories ORDER BY account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.LoanHistory{}
	for rows.Next() {
		history, err := scanLoanHistory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *history)
	}
	return out, rows.Err()
}

func (s *Store) UpsertLoanHistory(ctx context.Context, h *domain.LoanHistory) error {
	changes, err := json.Marshal(h.RateChanges)
	if err != nil {
		return err
	}
	prepayments, err := json.Marshal(h.Prepayments)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO loan_histories(`+loanHistoryColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			original_principal_cents = excluded.original_principal_cents,
			first_payment_date = excluded.first_payment_date,
			total_periods = excluded.total_periods,
			method = excluded.method,
			initial_annual_rate = excluded.initial_annual_rate,
			rate_changes = excluded.rate_changes,
			prepayments = excluded.prepayments,
			updated_at = excluded.updated_at`,
		h.AccountID, h.OriginalPrincipalCents, h.FirstPaymentDate, h.TotalPeriods,
		h.Method, h.InitialAnnualRate, string(changes), string(prepayments),
		formatTime(h.CreatedAt), formatTime(h.UpdatedAt))
	return err
}

func (s *Store) DeleteLoan(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM loans WHERE account_id = ?`, accountID)
	return err
}

func (s *Store) DeleteLoanHistory(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM loan_histories WHERE account_id = ?`, accountID)
	return err
}

func scanLoanHistory(row rowScanner) (*domain.LoanHistory, error) {
	var h domain.LoanHistory
	var changes, prepayments, created, updated string
	if err := row.Scan(&h.AccountID, &h.OriginalPrincipalCents, &h.FirstPaymentDate, &h.TotalPeriods,
		&h.Method, &h.InitialAnnualRate, &changes, &prepayments, &created, &updated); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(changes), &h.RateChanges); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(prepayments), &h.Prepayments); err != nil {
		return nil, err
	}
	h.CreatedAt = parseTime(created)
	h.UpdatedAt = parseTime(updated)
	return &h, nil
}
