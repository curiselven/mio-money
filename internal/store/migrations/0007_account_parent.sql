ALTER TABLE accounts ADD COLUMN parent_id TEXT REFERENCES accounts(id);

CREATE INDEX accounts_by_parent ON accounts(parent_id);
