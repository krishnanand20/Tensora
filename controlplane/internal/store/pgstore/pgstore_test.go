package pgstore

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pressly/goose/v3"

	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/store"
	"github.com/tensora/tensora/controlplane/internal/store/storetest"
	"github.com/tensora/tensora/controlplane/migrations"
)

func TestConformance(t *testing.T) {
	url := os.Getenv("TENSORA_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TENSORA_TEST_DATABASE_URL to run Postgres store tests (make test-cp-pg)")
	}
	ctx := context.Background()
	if _, err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	storetest.Run(t, func(t *testing.T) store.Store {
		st, err := Open(ctx, url)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(st.Close)
		if _, err := st.pool.Exec(ctx, `TRUNCATE idempotency_keys, usage_records, workload_events, workloads, machines, quotas, tenants`); err != nil {
			t.Fatal(err)
		}
		return st
	})
}

func TestSchemaMatchesSQLite(t *testing.T) {
	url := os.Getenv("TENSORA_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TENSORA_TEST_DATABASE_URL to run Postgres store tests (make test-cp-pg)")
	}
	ctx := context.Background()
	if _, err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	st, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	columns := map[string][]string{}
	rows, err := st.pool.Query(ctx, `SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name <> 'goose_db_version'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatal(err)
		}
		columns[table] = append(columns[table], column)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	// Indexes backing primary keys and unique constraints are left out, as in SQLite.
	rows, err = st.pool.Query(ctx, `SELECT i.indexname FROM pg_indexes i
		WHERE i.schemaname = current_schema() AND i.tablename <> 'goose_db_version'
		  AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conname = i.indexname)`)
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	storetest.CheckSchema(t, columns, indexes)
}

// Upgrading a database with data: workloads admitted before migration 00002 are backfilled as
// placed on the class they requested, and everything lands in region local. It runs in a
// throwaway schema, so the test database's own tables are untouched.
func TestUpgradeBackfillsPlacement(t *testing.T) {
	base := os.Getenv("TENSORA_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("set TENSORA_TEST_DATABASE_URL to run Postgres store tests (make test-cp-pg)")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	schema := "upgrade_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") })
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	url := base + sep + "search_path=" + schema

	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO tenants (id, slug, name, tier, created_at) VALUES ('00000000-0000-0000-0000-0000000000a1', 'acme', 'Acme', 'trusted', now());
		INSERT INTO workloads (id, tenant_id, name, kind, state, image_id, gpu_class, gpu_count, created_by, created_at, updated_at) VALUES
			('00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-0000000000a1', 'run', 'notebook', 'running', 'i', 'l40s-48gb', 1, 's', now(), now()),
			('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-0000000000a1', 'wait', 'notebook', 'queued', 'i', 'l40s-48gb', 1, 's', now(), now());`)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := Migrate(ctx, url); err != nil || n == 0 {
		t.Fatalf("upgrade: applied %d, %v", n, err)
	}

	st, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	err = st.InTx(ctx, func(q store.Queries) error {
		ws, err := q.WorkloadsInStates(ctx, domain.StateRunning, domain.StateQueued)
		if err != nil {
			return err
		}
		placed := map[string]string{}
		for _, w := range ws {
			if w.Region != domain.RegionLocal {
				t.Errorf("%s: region %q, want local", w.Name, w.Region)
			}
			placed[w.Name] = w.PlacedClass
		}
		if placed["run"] != "l40s-48gb" || placed["wait"] != "" {
			t.Errorf("placed classes after upgrade = %v; want the running one placed and the queued one not", placed)
		}
		alloc, err := q.GPUsAllocated(ctx)
		if err != nil || alloc["l40s-48gb"] != 1 {
			t.Errorf("GPUsAllocated after upgrade = %v, %v; the running workload must still hold its GPU", alloc, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
