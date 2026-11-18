package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/curiselven/mio-money/internal/domain"
)

func (s *Store) InsertSnapshot(ctx context.Context, sn *domain.Snapshot) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO snapshots
		(id, account_id, as_of, status, source, source_note, currency, cash_cents, created_at, confirmed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sn.ID, sn.AccountID, sn.AsOf, sn.Status, sn.Source, sn.SourceNote, sn.Currency,
		nullableCents(sn.CashCents), formatTime(sn.CreatedAt), nullableTime(sn.ConfirmedAt)); err != nil {
		return err
	}
	for i, it := range sn.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO snapshot_items
			(snapshot_id, position, name, code, quantity, market_value_cents, cost_cents, note)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			sn.ID, i, it.Name, it.Code, it.Quantity, it.MarketValueCents, nullableCents(it.CostCents), it.Note); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetSnapshot(ctx context.Context, id string) (*domain.Snapshot, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, account_id, as_of, status, source, source_note, currency,
		cash_cents, created_at, confirmed_at FROM snapshots WHERE id = ?`, id)
	sn, err := scanSnapshot(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("snapshot %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	items, err := s.loadItems(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	sn.Items = items[id]
	return sn, nil
}

func (s *Store) UpdateSnapshotStatus(ctx context.Context, id string, status domain.SnapshotStatus, confirmedAt *time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE snapshots SET status = ?, confirmed_at = ? WHERE id = ?`,
		status, nullableTime(confirmedAt), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("snapshot %s: %w", id, ErrNotFound)
	}
	return nil
}

// LastConfirmedBefore returns the latest confirmed snapshot with as_of <= date.
// Empty date means no upper bound.
func (s *Store) LastConfirmedBefore(ctx context.Context, accountID, date string) (*domain.Snapshot, error) {
	q := `SELECT id, account_id, as_of, status, source, source_note, currency, cash_cents, created_at, confirmed_at
		FROM snapshots WHERE account_id = ? AND status = 'confirmed'`
	args := []any{accountID}
	if date != "" {
		q += ` AND as_of <= ?`
		args = append(args, date)
	}
	q += ` ORDER BY as_of DESC, confirmed_at DESC, rowid DESC LIMIT 1`
	sn, err := scanSnapshot(s.db.QueryRowContext(ctx, q, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items, err := s.loadItems(ctx, []string{sn.ID})
	if err != nil {
		return nil, err
	}
	sn.Items = items[sn.ID]
	return sn, nil
}

// LastConfirmed returns the most recently confirmed snapshot of an account,
// ordered by confirmation time (this is what undo_snapshot reverts).
func (s *Store) LastConfirmed(ctx context.Context, accountID string) (*domain.Snapshot, error) {
	sn, err := scanSnapshot(s.db.QueryRowContext(ctx, `SELECT id, account_id, as_of, status, source, source_note, currency,
		cash_cents, created_at, confirmed_at FROM snapshots
		WHERE account_id = ? AND status = 'confirmed'
		ORDER BY confirmed_at DESC, rowid DESC LIMIT 1`, accountID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items, err := s.loadItems(ctx, []string{sn.ID})
	if err != nil {
		return nil, err
	}
	sn.Items = items[sn.ID]
	return sn, nil
}

// LatestConfirmedPerAccount returns the current confirmed snapshot of every
// account, optionally bounded by as_of.
func (s *Store) LatestConfirmedPerAccount(ctx context.Context, asOf string) ([]*domain.Snapshot, error) {
	q := `SELECT s.id, s.account_id, s.as_of, s.status, s.source, s.source_note, s.currency,
			s.cash_cents, s.created_at, s.confirmed_at
		FROM snapshots s
		WHERE s.status = 'confirmed'`
	args := []any{}
	if asOf != "" {
		q += ` AND s.as_of <= ?`
		args = append(args, asOf)
	}
	q += ` AND NOT EXISTS (
			SELECT 1 FROM snapshots n
			WHERE n.account_id = s.account_id AND n.status = 'confirmed'`
	if asOf != "" {
		q += ` AND n.as_of <= ?`
		args = append(args, asOf)
	}
	q += ` AND (n.as_of > s.as_of OR (n.as_of = s.as_of AND
			       (n.confirmed_at > s.confirmed_at OR (n.confirmed_at = s.confirmed_at AND n.rowid > s.rowid))))
		)`
	q += ` ORDER BY s.account_id`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var out []*domain.Snapshot
	for rows.Next() {
		sn, err := scanSnapshot(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, sn)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	ids := make([]string, len(out))
	for i, sn := range out {
		ids[i] = sn.ID
	}
	items, err := s.loadItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, sn := range out {
		sn.Items = items[sn.ID]
	}
	return out, nil
}

// ListConfirmed returns confirmed snapshots of an account (or all accounts
// when accountID is empty) with as_of in [from, to]. Empty bounds are open.
func (s *Store) ListConfirmed(ctx context.Context, accountID, from, to string) ([]*domain.Snapshot, error) {
	q := `SELECT s.id, s.account_id, s.as_of, s.status, s.source, s.source_note, s.currency,
			s.cash_cents, s.created_at, s.confirmed_at
		FROM snapshots s WHERE s.status = 'confirmed'`
	args := []any{}
	if accountID != "" {
		q += ` AND s.account_id = ?`
		args = append(args, accountID)
	}
	if from != "" {
		q += ` AND s.as_of >= ?`
		args = append(args, from)
	}
	if to != "" {
		q += ` AND s.as_of <= ?`
		args = append(args, to)
	}
	q += ` ORDER BY s.as_of, s.confirmed_at`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var out []*domain.Snapshot
	for rows.Next() {
		sn, err := scanSnapshot(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, sn)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(out) == 0 {
		return []*domain.Snapshot{}, nil
	}
	ids := make([]string, len(out))
	for i, sn := range out {
		ids[i] = sn.ID
	}
	items, err := s.loadItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, sn := range out {
		sn.Items = items[sn.ID]
	}
	return out, nil
}

func (s *Store) ListPending(ctx context.Context) ([]*domain.Snapshot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id, s.account_id, s.as_of, s.status, s.source, s.source_note, s.currency,
			s.cash_cents, s.created_at, s.confirmed_at
		FROM snapshots s WHERE s.status = 'pending' ORDER BY s.created_at`)
	if err != nil {
		return nil, err
	}
	var out []*domain.Snapshot
	for rows.Next() {
		sn, err := scanSnapshot(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, sn)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	ids := make([]string, len(out))
	for i, sn := range out {
		ids[i] = sn.ID
	}
	items, err := s.loadItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, sn := range out {
		sn.Items = items[sn.ID]
	}
	return out, nil
}

func (s *Store) AddAudit(ctx context.Context, e domain.AuditEntry) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_log(action, account_id, snapshot_id, detail, created_at)
		VALUES (?, ?, ?, ?, ?)`, e.Action, e.AccountID, e.SnapshotID, e.Detail, formatTime(e.CreatedAt))
	return err
}

func (s *Store) loadItems(ctx context.Context, snapshotIDs []string) (map[string][]domain.Item, error) {
	out := map[string][]domain.Item{}
	if len(snapshotIDs) == 0 {
		return out, nil
	}
	placeholders := strings.Repeat("?,", len(snapshotIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(snapshotIDs))
	for i, id := range snapshotIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT snapshot_id, name, code, quantity, market_value_cents, cost_cents, note
		FROM snapshot_items WHERE snapshot_id IN (`+placeholders+`) ORDER BY snapshot_id, position`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var snapshotID string
		var it domain.Item
		var cost sql.NullInt64
		if err := rows.Scan(&snapshotID, &it.Name, &it.Code, &it.Quantity, &it.MarketValueCents, &cost, &it.Note); err != nil {
			return nil, err
		}
		it.CostCents = centsOf(cost)
		out[snapshotID] = append(out[snapshotID], it)
	}
	return out, rows.Err()
}

func scanSnapshot(row rowScanner) (*domain.Snapshot, error) {
	var sn domain.Snapshot
	var cash sql.NullInt64
	var created string
	var confirmed sql.NullString
	if err := row.Scan(&sn.ID, &sn.AccountID, &sn.AsOf, &sn.Status, &sn.Source, &sn.SourceNote,
		&sn.Currency, &cash, &created, &confirmed); err != nil {
		return nil, err
	}
	sn.CashCents = centsOf(cash)
	sn.CreatedAt = parseTime(created)
	if confirmed.Valid {
		t := parseTime(confirmed.String)
		sn.ConfirmedAt = &t
	}
	sn.Items = []domain.Item{}
	return &sn, nil
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}
