-- The SQLite version of ../postgres/00005_agent_inventory_and_drivers.sql.

-- +goose Up
ALTER TABLE machines ADD COLUMN cpus INTEGER NOT NULL DEFAULT 0 CHECK (cpus >= 0);
ALTER TABLE machines ADD COLUMN memory_mib INTEGER NOT NULL DEFAULT 0 CHECK (memory_mib >= 0);
ALTER TABLE workloads ADD COLUMN driver TEXT;
CREATE INDEX workloads_machine_idx ON workloads (machine_id) WHERE machine_id IS NOT NULL;

-- +goose Down
DROP INDEX workloads_machine_idx;
ALTER TABLE workloads DROP COLUMN driver;
ALTER TABLE machines DROP COLUMN memory_mib;
ALTER TABLE machines DROP COLUMN cpus;
