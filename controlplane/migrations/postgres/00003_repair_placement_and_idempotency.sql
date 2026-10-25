-- Repairs to 00002 found in review (2026-10-24). A new migration, because 00002 was already applied.

-- +goose Up
-- 00002 judged "was it admitted?" by each workload's state at upgrade time, which is wrong for one
-- that ran and was waiting again after a restart, or one deleted before it was ever admitted. The
-- history says it exactly: a workload was admitted if it ever moved to provisioning, and before
-- 00002 it always ran on the class it requested.
UPDATE workloads w SET placed_class = w.gpu_class
WHERE w.placed_class IS NULL
  AND EXISTS (SELECT 1 FROM workload_events e WHERE e.workload_id = w.id AND e.to_state = 'provisioning');
UPDATE workloads w SET placed_class = NULL
WHERE w.placed_class IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM workload_events e WHERE e.workload_id = w.id AND e.to_state = 'provisioning');

-- Usage written with an empty class because of that gap ran on the workload's requested class.
-- A one-time repair: usage records are otherwise never updated.
UPDATE usage_records u SET gpu_class = w.gpu_class
FROM workloads w
WHERE w.id = u.workload_id AND u.gpu_class = '';

-- An idempotency key belongs to the person who sent it, not just their tenant, so nobody can replay
-- someone else's response. The table has never held data (it's used from Phase 2), so it's rebuilt.
-- Never store a response that carries a secret, such as a new API token: store a reference instead.
DROP TABLE idempotency_keys;
CREATE TABLE idempotency_keys (
    tenant_id       uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    subject         text        NOT NULL,
    idempotency_key text        NOT NULL CHECK (octet_length(idempotency_key) BETWEEN 1 AND 255),
    request_hash    text        NOT NULL,
    response_status integer     NOT NULL,
    response_body   bytea       NOT NULL,
    created_at      timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, subject, idempotency_key)
);
CREATE INDEX idempotency_keys_created_idx ON idempotency_keys (created_at);

-- The shipped endpoint image was renamed from vllm to openai-endpoint when images gained a runtime
-- per platform (roadmap step 4), so waiting and stopped endpoints keep working after the upgrade.
UPDATE workloads SET image_id = 'openai-endpoint' WHERE kind = 'endpoint' AND image_id = 'vllm';

-- +goose Down
-- The data repairs stay: undoing them would only bring the wrong values back.
DROP TABLE idempotency_keys;
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
