// Package memstore is an in-memory Store for `tensora dev` and tests. Transactions are
// serialized by one mutex and rolled back by restoring a snapshot.
package memstore

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/store"
)

type Store struct {
	mu sync.Mutex
	d  data
}

type quotaKey struct {
	tenant uuid.UUID
	class  string
}

type usageKey struct {
	workload uuid.UUID
	period   time.Time
}

type data struct {
	tenants     map[uuid.UUID]domain.Tenant
	quotas      map[quotaKey]domain.Quota
	workloads   map[uuid.UUID]domain.Workload
	events      []domain.Event
	usage       map[usageKey]domain.UsageRecord
	nextEventID int64
}

func New() *Store {
	return &Store{d: data{
		tenants:   map[uuid.UUID]domain.Tenant{},
		quotas:    map[quotaKey]domain.Quota{},
		workloads: map[uuid.UUID]domain.Workload{},
		usage:     map[usageKey]domain.UsageRecord{},
	}}
}

func (s *Store) InTx(_ context.Context, fn func(store.Queries) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := s.d.clone()
	if err := fn(&queries{d: &s.d}); err != nil {
		s.d = snapshot
		return err
	}
	return nil
}

func (s *Store) Ping(context.Context) error { return nil }
func (s *Store) Close()                     {}

func (d *data) clone() data {
	c := *d
	c.tenants = maps.Clone(d.tenants)
	c.quotas = maps.Clone(d.quotas)
	c.workloads = make(map[uuid.UUID]domain.Workload, len(d.workloads))
	for id, w := range d.workloads {
		c.workloads[id] = cloneWorkload(w)
	}
	c.events = slices.Clone(d.events)
	c.usage = maps.Clone(d.usage)
	return c
}

func cloneWorkload(w domain.Workload) domain.Workload {
	w.Params = maps.Clone(w.Params)
	return w
}

func sortWorkloads(ws []domain.Workload) {
	slices.SortFunc(ws, func(a, b domain.Workload) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID.String(), b.ID.String()))
	})
}

type queries struct{ d *data }

func (q *queries) CreateTenant(_ context.Context, t domain.Tenant) error {
	for _, existing := range q.d.tenants {
		if existing.Slug == t.Slug {
			return domain.Errorf(domain.CodeConflict, "tenant %q already exists", t.Slug)
		}
	}
	q.d.tenants[t.ID] = t
	return nil
}

func (q *queries) TenantBySlug(_ context.Context, slug string) (domain.Tenant, error) {
	for _, t := range q.d.tenants {
		if t.Slug == slug {
			return t, nil
		}
	}
	return domain.Tenant{}, store.NotFound("tenant")
}

func (q *queries) TenantByID(_ context.Context, id uuid.UUID) (domain.Tenant, error) {
	if t, ok := q.d.tenants[id]; ok {
		return t, nil
	}
	return domain.Tenant{}, store.NotFound("tenant")
}

func (q *queries) SetQuota(_ context.Context, quota domain.Quota) error {
	if _, ok := q.d.tenants[quota.TenantID]; !ok {
		return store.NotFound("tenant")
	}
	q.d.quotas[quotaKey{quota.TenantID, quota.GPUClass}] = quota
	return nil
}

func (q *queries) Quotas(_ context.Context, tenantID uuid.UUID) ([]domain.Quota, error) {
	var out []domain.Quota
	for k, v := range q.d.quotas {
		if k.tenant == tenantID {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(a, b domain.Quota) int { return cmp.Compare(a.GPUClass, b.GPUClass) })
	return out, nil
}

func (q *queries) LockQuota(_ context.Context, tenantID uuid.UUID, gpuClass string) (int, error) {
	return q.d.quotas[quotaKey{tenantID, gpuClass}].MaxGPUs, nil
}

func (q *queries) GPUsHeld(_ context.Context, tenantID uuid.UUID, gpuClass string) (int, error) {
	n := 0
	for _, w := range q.d.workloads {
		if w.TenantID == tenantID && w.GPUClass == gpuClass && w.State.HoldsQuota() {
			n += w.GPUCount
		}
	}
	return n, nil
}

func (q *queries) CreateWorkload(_ context.Context, w domain.Workload) error {
	if _, ok := q.d.tenants[w.TenantID]; !ok {
		return store.NotFound("tenant")
	}
	if _, ok := q.d.workloads[w.ID]; ok {
		return domain.Errorf(domain.CodeConflict, "workload %s already exists", w.ID)
	}
	q.d.workloads[w.ID] = cloneWorkload(w)
	return nil
}

func (q *queries) GetWorkload(_ context.Context, tenantID, id uuid.UUID) (domain.Workload, error) {
	w, ok := q.d.workloads[id]
	if !ok || w.TenantID != tenantID || w.State == domain.StateDeleted {
		return domain.Workload{}, store.NotFound("workload")
	}
	return cloneWorkload(w), nil
}

func (q *queries) GetWorkloadHistory(_ context.Context, tenantID, id uuid.UUID) (domain.Workload, error) {
	w, ok := q.d.workloads[id]
	if !ok || w.TenantID != tenantID {
		return domain.Workload{}, store.NotFound("workload")
	}
	return cloneWorkload(w), nil
}

func (q *queries) ListWorkloads(_ context.Context, tenantID uuid.UUID) ([]domain.Workload, error) {
	return q.listWorkloads(tenantID, false), nil
}

func (q *queries) ListWorkloadHistory(_ context.Context, tenantID uuid.UUID) ([]domain.Workload, error) {
	return q.listWorkloads(tenantID, true), nil
}

func (q *queries) listWorkloads(tenantID uuid.UUID, includeDeleted bool) []domain.Workload {
	out := []domain.Workload{}
	for _, w := range q.d.workloads {
		if w.TenantID == tenantID && (includeDeleted || w.State != domain.StateDeleted) {
			out = append(out, cloneWorkload(w))
		}
	}
	sortWorkloads(out)
	return out
}

func (q *queries) UpdateWorkloadState(_ context.Context, id uuid.UUID, from, to domain.State, at time.Time) error {
	w, ok := q.d.workloads[id]
	if !ok {
		return store.NotFound("workload")
	}
	if w.State != from {
		return store.StaleState(id, from)
	}
	w.State, w.UpdatedAt = to, at
	q.d.workloads[id] = w
	return nil
}

func (q *queries) WorkloadsInStates(_ context.Context, states ...domain.State) ([]domain.Workload, error) {
	var out []domain.Workload
	for _, w := range q.d.workloads {
		if slices.Contains(states, w.State) {
			out = append(out, cloneWorkload(w))
		}
	}
	sortWorkloads(out)
	return out, nil
}

func (q *queries) GPUsAllocated(context.Context) (map[string]int, error) {
	out := map[string]int{}
	for _, w := range q.d.workloads {
		if w.State.HoldsCapacity() {
			out[w.GPUClass] += w.GPUCount
		}
	}
	return out, nil
}

func (q *queries) WorkloadsForMetering(_ context.Context, since time.Time) ([]domain.Workload, error) {
	recent := map[uuid.UUID]bool{}
	for _, e := range q.d.events {
		if !e.At.Before(since) {
			recent[e.WorkloadID] = true
		}
	}
	var out []domain.Workload
	for _, w := range q.d.workloads {
		if w.State.Metered() || recent[w.ID] {
			out = append(out, cloneWorkload(w))
		}
	}
	sortWorkloads(out)
	return out, nil
}

func (q *queries) AppendEvent(_ context.Context, e domain.Event) error {
	if _, ok := q.d.workloads[e.WorkloadID]; !ok {
		return store.NotFound("workload")
	}
	q.d.nextEventID++
	e.ID = q.d.nextEventID
	q.d.events = append(q.d.events, e)
	return nil
}

func (q *queries) Events(_ context.Context, workloadID uuid.UUID) ([]domain.Event, error) {
	var out []domain.Event
	for _, e := range q.d.events {
		if e.WorkloadID == workloadID {
			out = append(out, e)
		}
	}
	slices.SortStableFunc(out, func(a, b domain.Event) int {
		return cmp.Or(a.At.Compare(b.At), cmp.Compare(a.ID, b.ID))
	})
	return out, nil
}

func (q *queries) EventsForWorkload(ctx context.Context, tenantID, workloadID uuid.UUID) ([]domain.Event, error) {
	if w, ok := q.d.workloads[workloadID]; !ok || w.TenantID != tenantID {
		return nil, nil
	}
	return q.Events(ctx, workloadID)
}

func (q *queries) InsertUsageIfAbsent(_ context.Context, r domain.UsageRecord) (bool, error) {
	k := usageKey{r.WorkloadID, r.PeriodStart.UTC()}
	if _, ok := q.d.usage[k]; ok {
		return false, nil
	}
	q.d.usage[k] = r
	return true, nil
}

func (q *queries) UsageForWorkload(_ context.Context, tenantID, workloadID uuid.UUID) ([]domain.UsageRecord, error) {
	var out []domain.UsageRecord
	for _, r := range q.d.usage {
		if r.TenantID == tenantID && r.WorkloadID == workloadID {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b domain.UsageRecord) int { return a.PeriodStart.Compare(b.PeriodStart) })
	return out, nil
}

func (q *queries) Usage(_ context.Context, tenantID uuid.UUID, from, to time.Time) ([]domain.UsageRecord, error) {
	var out []domain.UsageRecord
	for _, r := range q.d.usage {
		if r.TenantID == tenantID && !r.PeriodStart.Before(from) && r.PeriodStart.Before(to) {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b domain.UsageRecord) int {
		return cmp.Or(a.PeriodStart.Compare(b.PeriodStart), cmp.Compare(a.WorkloadID.String(), b.WorkloadID.String()))
	})
	return out, nil
}
