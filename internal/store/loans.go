package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/curiselven/mio-money/internal/domain"
)

const loanColumns = `account_id, anchor_date, anchor_principal_cents, annual_rate, method,
	payment_day, remaining_periods, created_at, updated_at`

// GetLoan returns the account's current terms, or nil when it has none.
func (s *Store) GetLoan(ctx context.Context, accountID string) (*domain.LoanTerms, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+loanColumns+` FROM loans WHERE account_id = ?`, accountID)
	loan, err := scanLoan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return loan, err
}

func (s *Store) ListLoans(ctx context.Context) ([]domain.LoanTerms, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+loanColumns+` FROM loans ORDER BY account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.LoanTerms{}
	for rows.Next() {
		loan, err := scanLoan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *loan)
	}
	return out, rows.Err()
}

func (s *Store) UpsertLoan(ctx context.Context, t *domain.LoanTerms) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO loans(`+loanColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			anchor_date = excluded.anchor_date,
			anchor_principal_cents = excluded.anchor_principal_cents,
			annual_rate = excluded.annual_rate,
			method = excluded.method,
			payment_day = excluded.payment_day,
			remaining_periods = excluded.remaining_periods,
			updated_at = excluded.updated_at`,
		t.AccountID, t.AnchorDate, t.AnchorPrincipalCents, t.AnnualRate, t.Method,
		t.PaymentDay, t.RemainingPeriods, formatTime(t.CreatedAt), formatTime(t.UpdatedAt))
	return err
}

// HasSnapshotAt reports whether any snapshot (whatever its status) exists for
// the account on that date.
func (s *Store) HasSnapshotAt(ctx context.Context, accountID, asOf string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM snapshots WHERE account_id = ? AND as_of = ?)`, accountID, asOf).Scan(&exists)
	return exists, err
}

func scanLoan(row rowScanner) (*domain.LoanTerms, error) {
	var t domain.LoanTerms
	var created, updated string
	if err := row.Scan(&t.AccountID, &t.AnchorDate, &t.AnchorPrincipalCents, &t.AnnualRate, &t.Method,
		&t.PaymentDay, &t.RemainingPeriods, &created, &updated); err != nil {
		return nil, err
	}
	t.CreatedAt = parseTime(created)
	t.UpdatedAt = parseTime(updated)
	return &t, nil
}
