package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/curiselven/mio-money/internal/domain"
)

func (s *Store) GetAccount(ctx context.Context, id string) (*domain.Account, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, platform, type, currency, owner, archived, created_at, updated_at
		FROM accounts WHERE id = ?`, id)
	a, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("account %s: %w", id, ErrNotFound)
	}
	return a, err
}

func (s *Store) UpsertAccount(ctx context.Context, a *domain.Account) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO accounts(id, name, platform, type, currency, owner, archived, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			platform = excluded.platform,
			type = excluded.type,
			currency = excluded.currency,
			owner = excluded.owner,
			archived = excluded.archived,
			updated_at = excluded.updated_at`,
		a.ID, a.Name, a.Platform, a.Type, a.Currency, a.Owner, a.Archived,
		formatTime(a.CreatedAt), formatTime(a.UpdatedAt))
	return err
}

func (s *Store) ListAccounts(ctx context.Context) ([]domain.Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, platform, type, currency, owner, archived, created_at, updated_at
		FROM accounts ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// LastConfirmedDates maps account id to the as_of date of its latest confirmed snapshot.
func (s *Store) LastConfirmedDates(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT account_id, MAX(as_of) FROM snapshots
		WHERE status = 'confirmed' GROUP BY account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, asOf string
		if err := rows.Scan(&id, &asOf); err != nil {
			return nil, err
		}
		out[id] = asOf
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAccount(row rowScanner) (*domain.Account, error) {
	var a domain.Account
	var created, updated string
	if err := row.Scan(&a.ID, &a.Name, &a.Platform, &a.Type, &a.Currency, &a.Owner, &a.Archived, &created, &updated); err != nil {
		return nil, err
	}
	a.CreatedAt = parseTime(created)
	a.UpdatedAt = parseTime(updated)
	return &a, nil
}
