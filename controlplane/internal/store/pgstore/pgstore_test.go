package pgstore

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/tensora/tensora/controlplane/internal/store"
	"github.com/tensora/tensora/controlplane/internal/store/storetest"
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
		if _, err := st.pool.Exec(ctx, `TRUNCATE usage_records, workload_events, workloads, quotas, tenants`); err != nil {
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
