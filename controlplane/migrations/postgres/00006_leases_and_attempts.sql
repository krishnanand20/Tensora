-- ADR-0016. Leases: one controller reconciles a region at a time; it takes the lease
-- "controller:<region>" and renews it, and another waits as a standby until it's free. Attempts:
-- each admission of a workload is a new attempt, so an agent runs an attempt at most once, and a
-- status answers only the attempt it was reported for.

-- +goose Up
CREATE TABLE leases (
    name       text        PRIMARY KEY,
    holder     text        NOT NULL,
    expires_at timestamptz NOT NULL
);
ALTER TABLE workloads ADD COLUMN attempt bigint NOT NULL DEFAULT 0 CHECK (attempt >= 0);

-- +goose Down
ALTER TABLE workloads DROP COLUMN attempt;
DROP TABLE leases;
