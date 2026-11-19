CREATE TABLE loan_histories (
    account_id               TEXT PRIMARY KEY REFERENCES accounts(id),
    original_principal_cents INTEGER NOT NULL CHECK (original_principal_cents > 0),
    first_payment_date       TEXT NOT NULL,
    total_periods            INTEGER NOT NULL CHECK (total_periods > 0),
    method                   TEXT NOT NULL CHECK (method IN ('equal_installment','equal_principal')),
    initial_annual_rate      TEXT NOT NULL,
    rate_changes             TEXT NOT NULL DEFAULT '[]',
    prepayments              TEXT NOT NULL DEFAULT '[]',
    created_at               TEXT NOT NULL,
    updated_at               TEXT NOT NULL
);
