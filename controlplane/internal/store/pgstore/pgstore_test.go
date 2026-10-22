package pgstore

import (
	"context"
	"os"
	"testing"

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
