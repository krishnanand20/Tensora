-- The SQLite version of ../postgres/00003_repair_placement_and_idempotency.sql.

-- +goose Up
UPDATE workloads SET placed_class = gpu_class
WHERE placed_class IS NULL
  AND EXISTS (SELECT 1 FROM workload_events e WHERE e.workload_id = workloads.id AND e.to_state = 'provisioning');
UPDATE workloads SET placed_class = NULL
WHERE placed_class IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM workload_events e WHERE e.workload_id = workloads.id AND e.to_state = 'provisioning');

UPDATE usage_records SET gpu_class = (SELECT w.gpu_class FROM workloads w WHERE w.id = usage_records.workload_id)
WHERE gpu_class = '';

DROP TABLE idempotency_keys;
CREATE TABLE idempotency_keys (
    tenant_id       TEXT    NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    subject         TEXT    NOT NULL,
    idempotency_key TEXT    NOT NULL CHECK (length(CAST(idempotency_key AS BLOB)) BETWEEN 1 AND 255),
    request_hash    TEXT    NOT NULL,
    response_status INTEGER NOT NULL,
    response_body   BLOB    NOT NULL,
    created_at      TEXT    NOT NULL,
    PRIMARY KEY (tenant_id, subject, idempotency_key)
) STRICT;
CREATE INDEX idempotency_keys_created_idx ON idempotency_keys (created_at);

-- The shipped endpoint image was renamed from vllm to openai-endpoint when images gained a runtime
-- per platform (roadmap step 4), so waiting and stopped endpoints keep working after the upgrade.
UPDATE workloads SET image_id = 'openai-endpoint' WHERE kind = 'endpoint' AND image_id = 'vllm';

-- +goose Down
-- The data repairs stay: undoing them would only bring the wrong values back.
DROP TABLE idempotency_keys;
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
