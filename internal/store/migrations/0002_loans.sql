-- SQLite cannot alter a CHECK constraint, so snapshots is rebuilt to allow the
-- loan_schedule source. migrations run with foreign_keys=OFF (see migrate).
CREATE TABLE snapshots_new (
    id           TEXT PRIMARY KEY,
    account_id   TEXT NOT NULL REFERENCES accounts(id),
    as_of        TEXT NOT NULL,
    status       TEXT NOT NULL CHECK (status IN ('pending','confirmed','rejected','undone')),
    source       TEXT NOT NULL CHECK (source IN ('screenshot','mcp','statement','manual','loan_schedule')),
    source_note  TEXT NOT NULL DEFAULT '',
    currency     TEXT NOT NULL DEFAULT '',
    cash_cents   INTEGER,
    created_at   TEXT NOT NULL,
    confirmed_at TEXT
);

INSERT INTO snapshots_new (id, account_id, as_of, status, source, source_note, currency, cash_cents, created_at, confirmed_at)
SELECT id, account_id, as_of, status, source, source_note, currency, cash_cents, created_at, confirmed_at
FROM snapshots ORDER BY rowid;

DROP TABLE snapshots;

ALTER TABLE snapshots_new RENAME TO snapshots;

CREATE INDEX snapshots_by_account ON snapshots(account_id, status, as_of);
CREATE INDEX snapshots_by_status ON snapshots(status, confirmed_at);

CREATE TABLE loans (
    account_id             TEXT PRIMARY KEY REFERENCES accounts(id),
    anchor_date            TEXT NOT NULL,
    anchor_principal_cents INTEGER NOT NULL CHECK (anchor_principal_cents > 0),
    annual_rate            TEXT NOT NULL,
    method                 TEXT NOT NULL CHECK (method IN ('equal_installment','equal_principal')),
    payment_day            INTEGER NOT NULL CHECK (payment_day BETWEEN 1 AND 28),
    remaining_periods      INTEGER NOT NULL CHECK (remaining_periods > 0),
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL
);
