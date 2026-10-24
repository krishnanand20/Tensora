// Package storetest is a conformance suite every store.Store implementation must pass.
package storetest

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/store"
)

// Run executes the suite. newStore must return an empty store.
func Run(t *testing.T, newStore func(t *testing.T) store.Store) {
	t.Run("TenantScoping", func(t *testing.T) { testTenantScoping(t, newStore(t)) })
	t.Run("StateUpdateIsCompareAndSwap", func(t *testing.T) { testCAS(t, newStore(t)) })
	t.Run("QuotaAndCapacityAccounting", func(t *testing.T) { testAccounting(t, newStore(t)) })
	t.Run("RollbackOnError", func(t *testing.T) { testRollback(t, newStore(t)) })
	t.Run("EventsAndUsage", func(t *testing.T) { testEventsAndUsage(t, newStore(t)) })
	t.Run("ErrorsAndCrossTenantReads", func(t *testing.T) { testErrorsAndCrossTenant(t, newStore(t)) })
}

// SchemaColumns and SchemaIndexes are every table's columns and every named index that both
// databases must have after all migrations (ADR-0012). sqlitestore and pgstore compare their
// database with them (CheckSchema), so a migration written for only one database fails a test.
var (
	SchemaColumns = map[string][]string{
		"tenants":         {"created_at", "id", "name", "slug", "tier"},
		"quotas":          {"gpu_class", "max_gpus", "tenant_id"},
		"workloads":       {"created_at", "created_by", "gpu_class", "gpu_count", "id", "image_id", "kind", "name", "params", "state", "tenant_id", "updated_at"},
		"workload_events": {"at", "from_state", "id", "reason", "to_state", "workload_id"},
		"usage_records":   {"created_at", "gpu_class", "gpu_count", "period_start", "seconds", "tenant_id", "workload_id"},
	}
	SchemaIndexes = []string{
		"usage_records_tenant_idx", "workload_events_at_idx", "workload_events_workload_idx",
		"workloads_active_state_idx", "workloads_tenant_idx",
	}
)

// CheckSchema compares a database's tables and columns, and its named indexes (not the ones
// backing primary keys or unique constraints), with SchemaColumns and SchemaIndexes.
func CheckSchema(t *testing.T, columns map[string][]string, indexes []string) {
	t.Helper()
	for table := range columns {
		slices.Sort(columns[table])
	}
	if !maps.EqualFunc(columns, SchemaColumns, slices.Equal) {
		t.Errorf("tables and columns = %v,\nwant %v", columns, SchemaColumns)
	}
	slices.Sort(indexes)
	if !slices.Equal(indexes, SchemaIndexes) {
		t.Errorf("indexes = %v, want %v", indexes, SchemaIndexes)
	}
}

var t0 = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

func tx(t *testing.T, st store.Store, fn func(q store.Queries) error) {
	t.Helper()
	if err := st.InTx(context.Background(), fn); err != nil {
		t.Fatal(err)
	}
}

func tenant(t *testing.T, st store.Store, slug string) domain.Tenant {
	t.Helper()
	tn := domain.Tenant{ID: uuid.New(), Slug: slug, Name: slug, Tier: domain.TierUntrusted, CreatedAt: t0}
	tx(t, st, func(q store.Queries) error { return q.CreateTenant(context.Background(), tn) })
	return tn
}

func workload(t *testing.T, st store.Store, tn domain.Tenant, state domain.State, gpus int) domain.Workload {
	t.Helper()
	w := domain.Workload{
		ID: uuid.New(), TenantID: tn.ID, Name: "w", Kind: domain.KindNotebook, State: state,
		ImageID: "img", GPUClass: "l40s-48gb", GPUCount: gpus, Params: map[string]string{},
		CreatedBy: "test", CreatedAt: t0, UpdatedAt: t0,
	}
	tx(t, st, func(q store.Queries) error { return q.CreateWorkload(context.Background(), w) })
	return w
}

func testTenantScoping(t *testing.T, st store.Store) {
	ctx := context.Background()
	a, b := tenant(t, st, "alpha"), tenant(t, st, "bravo")
	wa := workload(t, st, a, domain.StateRunning, 1)

	tx(t, st, func(q store.Queries) error {
		if _, err := q.GetWorkload(ctx, b.ID, wa.ID); !domain.IsNotFound(err) {
			t.Errorf("tenant B read tenant A's workload: err=%v", err)
		}
		list, err := q.ListWorkloads(ctx, b.ID)
		if err != nil || len(list) != 0 {
			t.Errorf("tenant B list = %v, %v; want empty", list, err)
		}
		got, err := q.GetWorkload(ctx, a.ID, wa.ID)
		if err != nil || got.ID != wa.ID {
			t.Errorf("owner read failed: %v", err)
		}
		return nil
	})

	// Deleted workloads vanish from normal reads but their history stays exportable by the owner only.
	tx(t, st, func(q store.Queries) error {
		return q.UpdateWorkloadState(ctx, wa.ID, domain.StateRunning, domain.StateDeleted, t0)
	})
	tx(t, st, func(q store.Queries) error {
		if _, err := q.GetWorkload(ctx, a.ID, wa.ID); !domain.IsNotFound(err) {
			t.Errorf("deleted workload still returned by GetWorkload: %v", err)
		}
		if got, err := q.GetWorkloadHistory(ctx, a.ID, wa.ID); err != nil || got.State != domain.StateDeleted {
			t.Errorf("owner history read = %+v, %v", got, err)
		}
		if _, err := q.GetWorkloadHistory(ctx, b.ID, wa.ID); !domain.IsNotFound(err) {
			t.Errorf("tenant B read tenant A's deleted workload history: err=%v", err)
		}
		if list, err := q.ListWorkloads(ctx, a.ID); err != nil || len(list) != 0 {
			t.Errorf("deleted workload still listed: %v, %v", list, err)
		}
		if list, err := q.ListWorkloadHistory(ctx, a.ID); err != nil || len(list) != 1 || list[0].State != domain.StateDeleted {
			t.Errorf("owner history list = %v, %v; want the deleted workload", list, err)
		}
		if list, err := q.ListWorkloadHistory(ctx, b.ID); err != nil || len(list) != 0 {
			t.Errorf("tenant B history list = %v, %v; want empty", list, err)
		}
		return nil
	})
}

func testCAS(t *testing.T, st store.Store) {
	ctx := context.Background()
	w := workload(t, st, tenant(t, st, "alpha"), domain.StatePending, 1)
	err := st.InTx(ctx, func(q store.Queries) error {
		return q.UpdateWorkloadState(ctx, w.ID, domain.StateRunning, domain.StateStopping, t0)
	})
	if c, _ := domain.CodeOf(err); c != domain.CodeConflict {
		t.Fatalf("stale update: err=%v, want conflict", err)
	}
	tx(t, st, func(q store.Queries) error {
		return q.UpdateWorkloadState(ctx, w.ID, domain.StatePending, domain.StateQueued, t0.Add(time.Minute))
	})
}

func testAccounting(t *testing.T, st store.Store) {
	ctx := context.Background()
	a := tenant(t, st, "alpha")
	tx(t, st, func(q store.Queries) error {
		return q.SetQuota(ctx, domain.Quota{TenantID: a.ID, GPUClass: "l40s-48gb", MaxGPUs: 4})
	})
	workload(t, st, a, domain.StateQueued, 1)  // quota yes, capacity no
	workload(t, st, a, domain.StateRunning, 2) // quota yes, capacity yes
	workload(t, st, a, domain.StateStopped, 4) // neither
	workload(t, st, a, domain.StateDeleting, 1)

	tx(t, st, func(q store.Queries) error {
		limit, err := q.LockQuota(ctx, a.ID, "l40s-48gb")
		if err != nil || limit != 4 {
			t.Errorf("LockQuota = %d, %v", limit, err)
		}
		if limit, _ := q.LockQuota(ctx, a.ID, "unset"); limit != 0 {
			t.Errorf("unset quota = %d, want 0", limit)
		}
		if held, _ := q.GPUsHeld(ctx, a.ID, "l40s-48gb"); held != 3 {
			t.Errorf("GPUsHeld = %d, want 3", held)
		}
		if alloc, _ := q.GPUsAllocated(ctx); alloc["l40s-48gb"] != 3 {
			t.Errorf("GPUsAllocated = %v, want 3 (running 2 + deleting 1)", alloc)
		}
		return nil
	})
}

func testRollback(t *testing.T, st store.Store) {
	ctx := context.Background()
	boom := errors.New("boom")
	err := st.InTx(ctx, func(q store.Queries) error {
		if err := q.CreateTenant(ctx, domain.Tenant{ID: uuid.New(), Slug: "ghost", Name: "g", Tier: domain.TierTrusted, CreatedAt: t0}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	tx(t, st, func(q store.Queries) error {
		if _, err := q.TenantBySlug(ctx, "ghost"); !domain.IsNotFound(err) {
			t.Errorf("tenant survived rollback: %v", err)
		}
		return nil
	})
}

func testEventsAndUsage(t *testing.T, st store.Store) {
	ctx := context.Background()
	a := tenant(t, st, "alpha")
	other := tenant(t, st, "bravo")
	w := workload(t, st, a, domain.StateRunning, 2)
	tx(t, st, func(q store.Queries) error {
		for i, to := range []domain.State{domain.StatePending, domain.StateProvisioning, domain.StateRunning} {
			if err := q.AppendEvent(ctx, domain.Event{WorkloadID: w.ID, To: to, Reason: "test", At: t0.Add(time.Duration(i) * time.Minute)}); err != nil {
				return err
			}
		}
		// A note: a timeline entry without a state change.
		return q.AppendEvent(ctx, domain.Event{WorkloadID: w.ID, From: domain.StateRunning, To: domain.StateRunning, Reason: "note", At: t0.Add(3 * time.Minute)})
	})
	tx(t, st, func(q store.Queries) error {
		evs, err := q.Events(ctx, w.ID)
		if err != nil || len(evs) != 4 || evs[2].To != domain.StateRunning || evs[0].From != "" || evs[3].From != domain.StateRunning {
			t.Errorf("events = %+v, %v", evs, err)
		}
		if mine, err := q.EventsForWorkload(ctx, a.ID, w.ID); err != nil || len(mine) != 4 {
			t.Errorf("EventsForWorkload(owner) = %+v, %v", mine, err)
		}
		if theirs, err := q.EventsForWorkload(ctx, other.ID, w.ID); err != nil || len(theirs) != 0 {
			t.Errorf("EventsForWorkload(other tenant) = %+v, %v; want none", theirs, err)
		}
		ws, err := q.WorkloadsForMetering(ctx, t0.Add(24*time.Hour))
		if err != nil || len(ws) != 1 {
			t.Errorf("running workload should be metered even without recent events: %v, %v", ws, err)
		}
		rec := domain.UsageRecord{WorkloadID: w.ID, PeriodStart: t0, TenantID: a.ID, GPUClass: "l40s-48gb", GPUCount: 2, Seconds: 3600}
		if ok, err := q.InsertUsageIfAbsent(ctx, rec); !ok || err != nil {
			t.Errorf("first insert = %v, %v", ok, err)
		}
		rec.Seconds = 1
		if ok, err := q.InsertUsageIfAbsent(ctx, rec); ok || err != nil {
			t.Errorf("duplicate insert = %v, %v; want no-op", ok, err)
		}
		got, err := q.Usage(ctx, a.ID, t0, t0.Add(time.Hour))
		if err != nil || len(got) != 1 || got[0].Seconds != 3600 || got[0].GPUSeconds() != 7200 {
			t.Errorf("usage = %+v, %v", got, err)
		}
		mine, err := q.UsageForWorkload(ctx, a.ID, w.ID)
		if err != nil || len(mine) != 1 || mine[0].WorkloadID != w.ID {
			t.Errorf("UsageForWorkload(owner) = %+v, %v", mine, err)
		}
		if theirs, err := q.UsageForWorkload(ctx, other.ID, w.ID); err != nil || len(theirs) != 0 {
			t.Errorf("UsageForWorkload(other tenant) = %+v, %v; want none", theirs, err)
		}
		return nil
	})
}

func testErrorsAndCrossTenant(t *testing.T, st store.Store) {
	ctx := context.Background()
	a, b := tenant(t, st, "alpha"), tenant(t, st, "bravo")
	w := workload(t, st, a, domain.StateRunning, 2)
	tx(t, st, func(q store.Queries) error {
		if err := q.SetQuota(ctx, domain.Quota{TenantID: a.ID, GPUClass: "l40s-48gb", MaxGPUs: 4}); err != nil {
			return err
		}
		_, err := q.InsertUsageIfAbsent(ctx, domain.UsageRecord{WorkloadID: w.ID, PeriodStart: t0, TenantID: a.ID,
			GPUClass: "l40s-48gb", GPUCount: 2, Seconds: 60})
		return err
	})

	orphan := w
	orphan.ID, orphan.TenantID = uuid.New(), uuid.New()
	cases := []struct {
		name string
		fn   func(q store.Queries) error
		want domain.Code
	}{
		{"duplicate tenant slug", func(q store.Queries) error {
			return q.CreateTenant(ctx, domain.Tenant{ID: uuid.New(), Slug: "alpha", Name: "x", Tier: domain.TierTrusted, CreatedAt: t0})
		}, domain.CodeConflict},
		{"duplicate workload", func(q store.Queries) error { return q.CreateWorkload(ctx, w) }, domain.CodeConflict},
		{"quota for a missing tenant", func(q store.Queries) error {
			return q.SetQuota(ctx, domain.Quota{TenantID: uuid.New(), GPUClass: "l40s-48gb", MaxGPUs: 1})
		}, domain.CodeNotFound},
		{"workload for a missing tenant", func(q store.Queries) error { return q.CreateWorkload(ctx, orphan) }, domain.CodeNotFound},
		{"event for a missing workload", func(q store.Queries) error {
			return q.AppendEvent(ctx, domain.Event{WorkloadID: uuid.New(), To: domain.StatePending, Reason: "x", At: t0})
		}, domain.CodeNotFound},
	}
	for _, c := range cases {
		// Each in its own transaction: Postgres aborts a transaction after an error.
		if code, _ := domain.CodeOf(st.InTx(ctx, c.fn)); code != c.want {
			t.Errorf("%s: code = %q, want %q", c.name, code, c.want)
		}
	}

	tx(t, st, func(q store.Queries) error {
		// The owner sees its quota, holdings, and usage, so the empty results below mean scoping,
		// not a broken query.
		if qs, err := q.Quotas(ctx, a.ID); err != nil || len(qs) != 1 {
			t.Errorf("owner quotas = %v, %v", qs, err)
		}
		if held, err := q.GPUsHeld(ctx, a.ID, "l40s-48gb"); err != nil || held != 2 {
			t.Errorf("owner GPUsHeld = %d, %v", held, err)
		}
		if u, err := q.Usage(ctx, a.ID, t0.Add(-time.Hour), t0.Add(time.Hour)); err != nil || len(u) != 1 {
			t.Errorf("owner usage = %v, %v", u, err)
		}
		if qs, err := q.Quotas(ctx, b.ID); err != nil || len(qs) != 0 {
			t.Errorf("tenant B quotas = %v, %v; want none", qs, err)
		}
		if limit, err := q.LockQuota(ctx, b.ID, "l40s-48gb"); err != nil || limit != 0 {
			t.Errorf("tenant B LockQuota = %d, %v; want 0", limit, err)
		}
		if held, err := q.GPUsHeld(ctx, b.ID, "l40s-48gb"); err != nil || held != 0 {
			t.Errorf("tenant B GPUsHeld = %d, %v; want 0", held, err)
		}
		if u, err := q.Usage(ctx, b.ID, t0.Add(-time.Hour), t0.Add(time.Hour)); err != nil || len(u) != 0 {
			t.Errorf("tenant B usage = %v, %v; want none", u, err)
		}
		return nil
	})
}
