-- Machines, regions, and placement (roadmap Phase 1, step 2; ADR-0009, 0010, 0013, 0014).

-- +goose Up
-- A computer that runs workloads through the node agent. One tenant owns it, and it belongs to
-- exactly one region.
CREATE TABLE machines (
    id           uuid        PRIMARY KEY,
    tenant_id    uuid        NOT NULL REFERENCES tenants (id),
    name         text        NOT NULL,
    region       text        NOT NULL,
    os           text        NOT NULL CHECK (os IN ('linux', 'darwin')),
    arch         text        NOT NULL CHECK (arch IN ('amd64', 'arm64')),
    accelerators jsonb       NOT NULL DEFAULT '[]',
    sharing      text        NOT NULL DEFAULT 'private' CHECK (sharing IN ('private', 'invite', 'community')),
    last_seen_at timestamptz,
    created_at   timestamptz NOT NULL,
    UNIQUE (tenant_id, name)
);
CREATE INDEX machines_region_idx ON machines (region);

-- Where a workload runs and on what. gpu_class stays the class the user requested; placed_class is
-- what it was admitted on, which differs only after an opt-in fallback.
ALTER TABLE workloads
    ADD COLUMN region           text    NOT NULL DEFAULT 'local',
    ADD COLUMN machine_id       uuid    REFERENCES machines (id),
    ADD COLUMN placed_class     text,
    ADD COLUMN fallback         jsonb   NOT NULL DEFAULT '[]',
    ADD COLUMN fallback_after_s integer NOT NULL DEFAULT 0 CHECK (fallback_after_s >= 0);
CREATE INDEX workloads_region_state_idx ON workloads (region, state);
-- Everything admitted before this migration ran on the class it requested (there were no fallbacks).
UPDATE workloads SET placed_class = gpu_class WHERE state NOT IN ('pending', 'queued');

-- Usage is accounted where, and on what, the work actually ran. gpu_class here is the placed class.
ALTER TABLE usage_records
    ADD COLUMN region     text NOT NULL DEFAULT 'local',
    ADD COLUMN machine_id uuid REFERENCES machines (id);

-- Responses to POSTs sent with an Idempotency-Key, so a retried request gets the first answer
-- instead of acting twice (ADR-0010; used from Phase 2).
CREATE TABLE idempotency_keys (
    tenant_id       uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    idempotency_key text        NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 255),
    request_hash    text        NOT NULL,
    response_status integer     NOT NULL,
    response_body   bytea       NOT NULL,
    created_at      timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, idempotency_key)
);
CREATE INDEX idempotency_keys_created_idx ON idempotency_keys (created_at);

-- +goose Down
DROP TABLE idempotency_keys;
ALTER TABLE usage_records DROP COLUMN machine_id, DROP COLUMN region;
DROP INDEX workloads_region_state_idx;
ALTER TABLE workloads
    DROP COLUMN fallback_after_s, DROP COLUMN fallback, DROP COLUMN placed_class,
    DROP COLUMN machine_id, DROP COLUMN region;
DROP TABLE machines;
