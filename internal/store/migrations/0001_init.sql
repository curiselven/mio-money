CREATE TABLE accounts (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    platform   TEXT NOT NULL DEFAULT '',
    type       TEXT NOT NULL CHECK (type IN ('cash','fund','stock','mixed','liability','other')),
    currency   TEXT NOT NULL DEFAULT 'CNY',
    owner      TEXT NOT NULL DEFAULT '',
    archived   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE snapshots (
    id           TEXT PRIMARY KEY,
    account_id   TEXT NOT NULL REFERENCES accounts(id),
    as_of        TEXT NOT NULL,
    status       TEXT NOT NULL CHECK (status IN ('pending','confirmed','rejected','undone')),
    source       TEXT NOT NULL CHECK (source IN ('screenshot','mcp','statement','manual')),
    source_note  TEXT NOT NULL DEFAULT '',
    currency     TEXT NOT NULL DEFAULT '',
    cash_cents   INTEGER,
    created_at   TEXT NOT NULL,
    confirmed_at TEXT
);

CREATE INDEX snapshots_by_account ON snapshots(account_id, status, as_of);
CREATE INDEX snapshots_by_status ON snapshots(status, confirmed_at);

CREATE TABLE snapshot_items (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id        TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    position           INTEGER NOT NULL,
    name               TEXT NOT NULL,
    code               TEXT NOT NULL DEFAULT '',
    quantity           TEXT NOT NULL DEFAULT '',
    market_value_cents INTEGER NOT NULL,
    cost_cents         INTEGER,
    note               TEXT NOT NULL DEFAULT ''
);

CREATE INDEX snapshot_items_by_snapshot ON snapshot_items(snapshot_id, position);

CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    action      TEXT NOT NULL,
    account_id  TEXT NOT NULL,
    snapshot_id TEXT NOT NULL,
    detail      TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL
);

CREATE INDEX audit_log_by_account ON audit_log(account_id, created_at);
