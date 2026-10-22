-- +goose Up
CREATE TABLE tenants (
    id         uuid PRIMARY KEY,
    slug       text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?$'),
    name       text NOT NULL,
    tier       text NOT NULL CHECK (tier IN ('trusted', 'untrusted')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE quotas (
    tenant_id uuid    NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    gpu_class text    NOT NULL,
    max_gpus  integer NOT NULL CHECK (max_gpus >= 0),
    PRIMARY KEY (tenant_id, gpu_class)
);

CREATE TABLE workloads (
    id         uuid PRIMARY KEY,
    tenant_id  uuid        NOT NULL REFERENCES tenants (id),
    name       text        NOT NULL,
    kind       text        NOT NULL CHECK (kind IN ('vm', 'notebook', 'endpoint')),
    state      text        NOT NULL,
    image_id   text        NOT NULL,
    gpu_class  text        NOT NULL,
    gpu_count  integer     NOT NULL CHECK (gpu_count > 0),
    params     jsonb       NOT NULL DEFAULT '{}',
    created_by text        NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX workloads_tenant_idx ON workloads (tenant_id, created_at);
CREATE INDEX workloads_active_state_idx ON workloads (state) WHERE state <> 'deleted';

-- Append-only: metering and audits depend on this history being immutable.
CREATE TABLE workload_events (
    id          bigserial PRIMARY KEY,
    workload_id uuid        NOT NULL REFERENCES workloads (id),
    from_state  text,
    to_state    text        NOT NULL,
    reason      text        NOT NULL,
    at          timestamptz NOT NULL
);
CREATE INDEX workload_events_workload_idx ON workload_events (workload_id, at, id);
CREATE INDEX workload_events_at_idx ON workload_events (at);

-- +goose StatementBegin
CREATE FUNCTION tensora_forbid_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER workload_events_append_only
    BEFORE UPDATE OR DELETE ON workload_events
    FOR EACH ROW EXECUTE FUNCTION tensora_forbid_mutation();

-- One row per workload per closed UTC hour. Written once by metering, never updated.
CREATE TABLE usage_records (
    workload_id  uuid        NOT NULL REFERENCES workloads (id),
    period_start timestamptz NOT NULL,
    tenant_id    uuid        NOT NULL REFERENCES tenants (id),
    gpu_class    text        NOT NULL,
    gpu_count    integer     NOT NULL CHECK (gpu_count > 0),
    seconds      integer     NOT NULL CHECK (seconds BETWEEN 0 AND 3600),
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workload_id, period_start)
);
CREATE INDEX usage_records_tenant_idx ON usage_records (tenant_id, period_start);

-- +goose Down
DROP TABLE usage_records;
DROP TRIGGER workload_events_append_only ON workload_events;
DROP TABLE workload_events;
DROP FUNCTION tensora_forbid_mutation();
DROP TABLE workloads;
DROP TABLE quotas;
DROP TABLE tenants;
