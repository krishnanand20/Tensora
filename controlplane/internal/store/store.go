// Package store defines persistence for business state. Implementations: memstore
// (dev/tests) and pgstore (Postgres). Both must pass the storetest conformance suite.
package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/domain"
)

type Store interface {
	// InTx runs fn in a transaction: committed if fn returns nil, rolled back otherwise.
	InTx(ctx context.Context, fn func(q Queries) error) error
	Ping(ctx context.Context) error
	Close()
}

// Queries is every operation available inside a transaction.
//
// Tenant-scoped methods take a tenantID and must never return another tenant's rows.
// System-wide methods are for the controller and metering only — never call them from
// request handlers (AGENTS.md rule 8).
type Queries interface {
	// Tenants and quotas
	CreateTenant(ctx context.Context, t domain.Tenant) error
	TenantBySlug(ctx context.Context, slug string) (domain.Tenant, error)
	TenantByID(ctx context.Context, id uuid.UUID) (domain.Tenant, error)
	SetQuota(ctx context.Context, q domain.Quota) error
	Quotas(ctx context.Context, tenantID uuid.UUID) ([]domain.Quota, error)
	// LockQuota returns the tenant's limit for a GPU class (0 if unset). In Postgres it
	// locks the quota row until the transaction ends, so concurrent creates serialize.
	LockQuota(ctx context.Context, tenantID uuid.UUID, gpuClass string) (int, error)
	// GPUsHeld sums gpu_count over the tenant's workloads whose state HoldsQuota.
	GPUsHeld(ctx context.Context, tenantID uuid.UUID, gpuClass string) (int, error)

	// Workloads, tenant-scoped. Deleted workloads are invisible.
	CreateWorkload(ctx context.Context, w domain.Workload) error
	GetWorkload(ctx context.Context, tenantID, id uuid.UUID) (domain.Workload, error)
	ListWorkloads(ctx context.Context, tenantID uuid.UUID) ([]domain.Workload, error)
	// UpdateWorkloadState is a compare-and-swap: it fails with CodeConflict unless the
	// workload is still in state from. Use service.Transition, not this, directly.
	UpdateWorkloadState(ctx context.Context, id uuid.UUID, from, to domain.State, at time.Time) error

	// Workloads, system-wide (controller / metering only).
	WorkloadsInStates(ctx context.Context, states ...domain.State) ([]domain.Workload, error)
	// GPUsAllocated sums gpu_count by GPU class over workloads whose state HoldsCapacity.
	GPUsAllocated(ctx context.Context) (map[string]int, error)
	// WorkloadsForMetering returns workloads that are billable now or had an event at or after since.
	WorkloadsForMetering(ctx context.Context, since time.Time) ([]domain.Workload, error)

	// Events (append-only) and usage.
	AppendEvent(ctx context.Context, e domain.Event) error
	Events(ctx context.Context, workloadID uuid.UUID) ([]domain.Event, error)
	// InsertUsageIfAbsent writes r unless a record for (workload, period) exists; reports whether it wrote.
	InsertUsageIfAbsent(ctx context.Context, r domain.UsageRecord) (bool, error)
	Usage(ctx context.Context, tenantID uuid.UUID, from, to time.Time) ([]domain.UsageRecord, error)
}

func NotFound(what string) error { return domain.Errorf(domain.CodeNotFound, "%s not found", what) }

func StaleState(id uuid.UUID, from domain.State) error {
	return domain.Errorf(domain.CodeConflict, "workload %s is no longer %s", id, from)
}

// StatesAsStrings converts states for SQL array parameters.
func StatesAsStrings(states []domain.State) []string {
	out := make([]string, len(states))
	for i, s := range states {
		out[i] = string(s)
	}
	return out
}
