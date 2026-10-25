-- The node agent (roadmap Phase 1, step 6; ADR-0016): what each machine's agent reports about its
-- CPUs and memory, the driver each workload placed on a machine runs with, and an index for the
-- agent backend, which reads a machine's workloads on every sync.

-- +goose Up
ALTER TABLE machines ADD COLUMN cpus integer NOT NULL DEFAULT 0 CHECK (cpus >= 0);
ALTER TABLE machines ADD COLUMN memory_mib bigint NOT NULL DEFAULT 0 CHECK (memory_mib >= 0);
ALTER TABLE workloads ADD COLUMN driver text;
CREATE INDEX workloads_machine_idx ON workloads (machine_id) WHERE machine_id IS NOT NULL;

-- +goose Down
DROP INDEX workloads_machine_idx;
ALTER TABLE workloads DROP COLUMN driver;
ALTER TABLE machines DROP COLUMN memory_mib;
ALTER TABLE machines DROP COLUMN cpus;
