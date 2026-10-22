// Package storetest is a conformance suite every store.Store implementation must pass.
package storetest

import (
	"context"
	"errors"
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
	w := workload(t, st, a, domain.StateRunning, 2)
	tx(t, st, func(q store.Queries) error {
		for i, to := range []domain.State{domain.StatePending, domain.StateProvisioning, domain.StateRunning} {
			if err := q.AppendEvent(ctx, domain.Event{WorkloadID: w.ID, To: to, Reason: "test", At: t0.Add(time.Duration(i) * time.Minute)}); err != nil {
				return err
			}
		}
		return nil
	})
	tx(t, st, func(q store.Queries) error {
		evs, err := q.Events(ctx, w.ID)
		if err != nil || len(evs) != 3 || evs[2].To != domain.StateRunning || evs[0].From != "" {
			t.Errorf("events = %+v, %v", evs, err)
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
		return nil
	})
}
