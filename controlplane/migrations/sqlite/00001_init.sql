-- The SQLite version of ../postgres/00001_init.sql (ADR-0012): the same tables, columns,
-- constraints, and indexes. STRICT tables enforce column types. UUIDs are text, and times are
-- fixed-width UTC text (2006-01-02T15:04:05.000000000Z), so text order is time order.

-- +goose Up
CREATE TABLE tenants (
    id         TEXT PRIMARY KEY,
    -- The same rule as Postgres's ^[a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?$, without regex support.
    -- length() and GLOB stop at a NUL byte, so the byte length must also match the text length.
    slug       TEXT NOT NULL UNIQUE CHECK (
                   length(CAST(slug AS BLOB)) BETWEEN 1 AND 40
                   AND length(slug) = length(CAST(slug AS BLOB))
                   AND slug NOT GLOB '*[^a-z0-9-]*'
                   AND slug NOT GLOB '-*'
                   AND slug NOT GLOB '*-'),
    name       TEXT NOT NULL,
    tier       TEXT NOT NULL CHECK (tier IN ('trusted', 'untrusted')),
    created_at TEXT NOT NULL
) STRICT;

CREATE TABLE quotas (
    tenant_id TEXT    NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    gpu_class TEXT    NOT NULL,
    max_gpus  INTEGER NOT NULL CHECK (max_gpus >= 0),
    PRIMARY KEY (tenant_id, gpu_class)
) STRICT;

CREATE TABLE workloads (
    id         TEXT    PRIMARY KEY,
    tenant_id  TEXT    NOT NULL REFERENCES tenants (id),
    name       TEXT    NOT NULL,
    kind       TEXT    NOT NULL CHECK (kind IN ('vm', 'notebook', 'endpoint')),
    state      TEXT    NOT NULL,
    image_id   TEXT    NOT NULL,
    gpu_class  TEXT    NOT NULL,
    gpu_count  INTEGER NOT NULL CHECK (gpu_count > 0),
    params     TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(params)),
    created_by TEXT    NOT NULL,
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL
) STRICT;
CREATE INDEX workloads_tenant_idx ON workloads (tenant_id, created_at);
CREATE INDEX workloads_active_state_idx ON workloads (state) WHERE state <> 'deleted';

-- Append-only: metering and audits depend on this history being immutable.
CREATE TABLE workload_events (
    id          INTEGER PRIMARY KEY,
    workload_id TEXT    NOT NULL REFERENCES workloads (id),
    from_state  TEXT,
    to_state    TEXT    NOT NULL,
    reason      TEXT    NOT NULL,
    at          TEXT    NOT NULL
) STRICT;
CREATE INDEX workload_events_workload_idx ON workload_events (workload_id, at, id);
CREATE INDEX workload_events_at_idx ON workload_events (at);

-- +goose StatementBegin
CREATE TRIGGER workload_events_no_update BEFORE UPDATE ON workload_events
BEGIN
    SELECT RAISE(ABORT, 'workload_events is append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER workload_events_no_delete BEFORE DELETE ON workload_events
BEGIN
    SELECT RAISE(ABORT, 'workload_events is append-only');
END;
-- +goose StatementEnd

-- One row per workload per closed UTC hour. Written once by metering, never updated.
CREATE TABLE usage_records (
    workload_id  TEXT    NOT NULL REFERENCES workloads (id),
    period_start TEXT    NOT NULL,
    tenant_id    TEXT    NOT NULL REFERENCES tenants (id),
    gpu_class    TEXT    NOT NULL,
    gpu_count    INTEGER NOT NULL CHECK (gpu_count > 0),
    seconds      INTEGER NOT NULL CHECK (seconds BETWEEN 0 AND 3600),
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now') || '000000Z'), -- 9 digits, like every time
    PRIMARY KEY (workload_id, period_start)
) STRICT;
CREATE INDEX usage_records_tenant_idx ON usage_records (tenant_id, period_start);

-- +goose Down
DROP TABLE usage_records;
DROP TRIGGER workload_events_no_delete;
DROP TRIGGER workload_events_no_update;
DROP TABLE workload_events;
DROP TABLE workloads;
DROP TABLE quotas;
DROP TABLE tenants;
