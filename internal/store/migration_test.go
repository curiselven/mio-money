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
	if version, err := st.SchemaVersion(ctx); err != nil || version != 7 {
		t.Fatalf("version = %d, err = %v", version, err)
	}
}

// TestDisbursementMigrationKeepsHistory builds a 0003 database with a history
// plan, then opens it so 0004 adds the column and 0005 backfills principal_rule.
func TestDisbursementMigrationKeepsHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v3.db")
	scripts := make([]string, 0, 3)
	for _, name := range []string{"0001_init.sql", "0002_loans.sql", "0003_loan_histories.sql"} {
		body, err := os.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		scripts = append(scripts, string(body))
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	for _, script := range scripts {
		if _, err := db.Exec(script); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, stmt := range []string{
		`INSERT INTO schema_version(version, applied_at) VALUES (1, '` + now + `'), (2, '` + now + `'), (3, '` + now + `')`,
		`INSERT INTO accounts(id, name, type, created_at, updated_at) VALUES ('acc_old', '旧房贷', 'liability', '` + now + `', '` + now + `')`,
		`INSERT INTO loan_histories(account_id, original_principal_cents, first_payment_date, total_periods, method,
			initial_annual_rate, rate_changes, prepayments, created_at, updated_at)
		 VALUES ('acc_old', 100000000, '2026-01-20', 360, 'equal_installment', '4.90',
			'[{"date":"2026-01-01","annual_rate":"4.70"},{"date":"2026-06-01","annual_rate":"4.50","principal_rule":"old_schedule"}]', '[]', '` + now + `', '` + now + `')`,
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
	history, err := st.GetLoanHistory(ctx, "acc_old")
	if err != nil || history == nil || history.TotalPeriods != 360 || len(history.RateChanges) != 2 {
		t.Fatalf("history = %+v, err = %v", history, err)
	}
	// 0005 backfills the missing rule and keeps an explicit one.
	if history.RateChanges[0].PrincipalRule != domain.PrincipalNewSchedule ||
		history.RateChanges[1].PrincipalRule != domain.PrincipalOldSchedule {
		t.Fatalf("rules = %+v", history.RateChanges)
	}
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var storedRule string
	if err := raw.QueryRow(`SELECT json_extract(rate_changes, '$[0].principal_rule')
		FROM loan_histories WHERE account_id = 'acc_old'`).Scan(&storedRule); err != nil {
		t.Fatal(err)
	}
	if storedRule != "new_schedule" {
		t.Fatalf("migration stored rule = %q", storedRule)
	}
	if history.DisbursementDate != "" {
		t.Fatalf("disbursement = %q", history.DisbursementDate)
	}
	history.DisbursementDate = "2025-12-15"
	history.UpdatedAt = time.Now().UTC()
	if err := st.UpsertLoanHistory(ctx, history); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetLoanHistory(ctx, "acc_old")
	if err != nil || updated == nil || updated.DisbursementDate != "2025-12-15" {
		t.Fatalf("updated = %+v, err = %v", updated, err)
	}
	if version, err := st.SchemaVersion(ctx); err != nil || version != 7 {
		t.Fatalf("version = %d, err = %v", version, err)
	}
}

func TestEfundSourceMigrationKeepsSnapshots(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v5.db")
	names := []string{
		"0001_init.sql", "0002_loans.sql", "0003_loan_histories.sql",
		"0004_loan_disbursement.sql", "0005_principal_rule.sql",
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, stmt := range []string{
		`INSERT INTO schema_version(version, applied_at) VALUES (1, '` + now + `'), (2, '` + now + `'),
			(3, '` + now + `'), (4, '` + now + `'), (5, '` + now + `')`,
		`INSERT INTO accounts(id, name, type, created_at, updated_at) VALUES ('acc_old', '旧基金账户', 'fund', '` + now + `', '` + now + `')`,
		`INSERT INTO snapshots(id, account_id, as_of, status, source, currency, cash_cents, created_at)
			VALUES ('snp_old', 'acc_old', '2026-01-01', 'confirmed', 'manual', 'CNY', 100, '` + now + `')`,
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
	if err != nil || len(snapshot.Items) != 1 {
		t.Fatalf("snapshot = %+v, err = %v", snapshot, err)
	}
	if err := st.InsertSnapshot(ctx, &domain.Snapshot{
		ID: "snp_efund", AccountID: "acc_old", AsOf: "2026-11-17", Status: domain.StatusConfirmed,
		Source: domain.SourceEfundImport, Currency: "CNY", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("insert efund snapshot: %v", err)
	}
	if version, err := st.SchemaVersion(ctx); err != nil || version != 7 {
		t.Fatalf("version = %d, err = %v", version, err)
	}
}

func TestAccountParentMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v6.db")
	names := []string{
		"0001_init.sql", "0002_loans.sql", "0003_loan_histories.sql",
		"0004_loan_disbursement.sql", "0005_principal_rule.sql", "0006_efund_import_source.sql",
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, stmt := range []string{
		`INSERT INTO schema_version(version, applied_at) VALUES (1, '` + now + `'), (2, '` + now + `'),
			(3, '` + now + `'), (4, '` + now + `'), (5, '` + now + `'), (6, '` + now + `')`,
		`INSERT INTO accounts(id, name, type, created_at, updated_at) VALUES ('acc_old', '旧账户', 'fund', '` + now + `', '` + now + `')`,
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
	old, err := st.GetAccount(ctx, "acc_old")
	if err != nil || old.ParentID != "" {
		t.Fatalf("old account = %+v, err=%v", old, err)
	}
	child := &domain.Account{
		ID: "acc_child", Name: "示例组合", ParentID: "acc_old", Type: domain.AccountFund,
		Currency: "CNY", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := st.UpsertAccount(ctx, child); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetAccount(ctx, "acc_child")
	if err != nil || got.ParentID != "acc_old" {
		t.Fatalf("child = %+v, err=%v", got, err)
	}
	if version, err := st.SchemaVersion(ctx); err != nil || version != 7 {
		t.Fatalf("version = %d, err = %v", version, err)
	}
}
