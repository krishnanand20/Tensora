-- The SQLite version of ../postgres/00006_leases_and_attempts.sql.

-- +goose Up
CREATE TABLE leases (
    name       TEXT PRIMARY KEY,
    holder     TEXT NOT NULL,
    expires_at TEXT NOT NULL
) STRICT;
ALTER TABLE workloads ADD COLUMN attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0);

-- +goose Down
ALTER TABLE workloads DROP COLUMN attempt;
DROP TABLE leases;
