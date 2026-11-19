package store_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/store"
)

// TestLoanMigrationKeepsExistingData builds a database at migration 1, then
// opens it with store.Open so 0002 rebuilds snapshots without losing rows.
func TestLoanMigrationKeepsExistingData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	init, err := os.ReadFile("migrations/0001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(init)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, stmt := range []string{
		`INSERT INTO schema_version(version, applied_at) VALUES (1, '` + now + `')`,
		`INSERT INTO accounts(id, name, type, created_at, updated_at) VALUES ('acc_old', '旧账户', 'liability', '` + now + `', '` + now + `')`,
		`INSERT INTO snapshots(id, account_id, as_of, status, source, currency, cash_cents, created_at)
			VALUES ('snp_old', 'acc_old', '2026-01-01', 'confirmed', 'manual', 'CNY', -1000, '` + now + `')`,
		`INSERT INTO snapshot_items(snapshot_id, position, name, market_value_cents) VALUES ('snp_old', 0, '旧持仓', 100)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	snapshot, err := st.GetSnapshot(ctx, "snp_old")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CashCents == nil || *snapshot.CashCents != -1000 || len(snapshot.Items) != 1 {
		t.Fatalf("snapshot after migration = %+v", snapshot)
	}
	cash := int64(-900)
	if err := st.InsertSnapshot(ctx, &domain.Snapshot{
		ID: "snp_loan", AccountID: "acc_old", AsOf: "2026-02-01", Status: domain.StatusConfirmed,
		Source: domain.SourceLoan, Currency: "CNY", CashCents: &cash, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("insert loan snapshot: %v", err)
	}
	has, err := st.HasSnapshotAt(ctx, "acc_old", "2026-02-01")
	if err != nil || !has {
		t.Fatalf("has snapshot = %v, err = %v", has, err)
	}
}

// TestLoanHistoryMigrationKeepsAnchorLoan builds a 0002 database with anchor
// terms, then opens it so 0003 runs on top without touching the loans row.
func TestLoanHistoryMigrationKeepsAnchorLoan(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v2.db")
	init, err := os.ReadFile("migrations/0001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	loans, err := os.ReadFile("migrations/0002_loans.sql")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(init)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(loans)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, stmt := range []string{
		`INSERT INTO schema_version(version, applied_at) VALUES (1, '` + now + `'), (2, '` + now + `')`,
		`INSERT INTO accounts(id, name, type, created_at, updated_at) VALUES ('acc_old', '旧房贷', 'liability', '` + now + `', '` + now + `')`,
		`INSERT INTO loans(account_id, anchor_date, anchor_principal_cents, annual_rate, method, payment_day, remaining_periods, created_at, updated_at)
		 VALUES ('acc_old', '2026-01-01', 100000000, '3.10', 'equal_installment', 20, 240, '` + now + `', '` + now + `')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	loan, err := st.GetLoan(ctx, "acc_old")
	if err != nil || loan == nil || loan.AnchorPrincipalCents != 100_000_000 {
		t.Fatalf("loan = %+v, err = %v", loan, err)
	}
	if history, err := st.GetLoanHistory(ctx, "acc_old"); err != nil || history != nil {
		t.Fatalf("history = %+v, err = %v", history, err)
	}
	if version, err := st.SchemaVersion(ctx); err != nil || version != 3 {
		t.Fatalf("version = %d, err = %v", version, err)
	}
}
