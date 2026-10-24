-- The SQLite version of ../postgres/00002_machines_and_placement.sql: the same tables, columns,
-- constraints, and indexes. Columns added to an existing table need a default in SQLite; the
-- defaults match Postgres.

-- +goose Up
CREATE TABLE machines (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants (id),
    name         TEXT NOT NULL,
    region       TEXT NOT NULL,
    os           TEXT NOT NULL CHECK (os IN ('linux', 'darwin')),
    arch         TEXT NOT NULL CHECK (arch IN ('amd64', 'arm64')),
    accelerators TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(accelerators)),
    sharing      TEXT NOT NULL DEFAULT 'private' CHECK (sharing IN ('private', 'invite', 'community')),
    last_seen_at TEXT,
    created_at   TEXT NOT NULL,
    UNIQUE (tenant_id, name)
) STRICT;
CREATE INDEX machines_region_idx ON machines (region);

ALTER TABLE workloads ADD COLUMN region TEXT NOT NULL DEFAULT 'local';
ALTER TABLE workloads ADD COLUMN machine_id TEXT REFERENCES machines (id);
ALTER TABLE workloads ADD COLUMN placed_class TEXT;
ALTER TABLE workloads ADD COLUMN fallback TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(fallback));
ALTER TABLE workloads ADD COLUMN fallback_after_s INTEGER NOT NULL DEFAULT 0 CHECK (fallback_after_s >= 0);
CREATE INDEX workloads_region_state_idx ON workloads (region, state);
-- Everything admitted before this migration ran on the class it requested (there were no fallbacks).
UPDATE workloads SET placed_class = gpu_class WHERE state NOT IN ('pending', 'queued');

ALTER TABLE usage_records ADD COLUMN region TEXT NOT NULL DEFAULT 'local';
ALTER TABLE usage_records ADD COLUMN machine_id TEXT REFERENCES machines (id);

CREATE TABLE idempotency_keys (
    tenant_id       TEXT    NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    idempotency_key TEXT    NOT NULL CHECK (length(CAST(idempotency_key AS BLOB)) BETWEEN 1 AND 255),
    request_hash    TEXT    NOT NULL,
    response_status INTEGER NOT NULL,
    response_body   BLOB    NOT NULL,
    created_at      TEXT    NOT NULL,
    PRIMARY KEY (tenant_id, idempotency_key)
) STRICT;
CREATE INDEX idempotency_keys_created_idx ON idempotency_keys (created_at);

-- +goose Down
-- SQLite can't drop a column that has a foreign key, so this migration can't be reverted in place.
-- The statement below fails on purpose, naming the way back: restore the backup taken before the
-- upgrade (`tensora backup`, planned).
SELECT * FROM "cannot_downgrade_sqlite_past_00002__restore_a_backup";
