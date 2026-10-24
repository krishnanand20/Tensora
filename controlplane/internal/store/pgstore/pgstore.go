// Package pgstore is the Postgres Store used outside local dev.
package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver, used only by goose
	"github.com/pressly/goose/v3"

	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/store"
	"github.com/tensora/tensora/controlplane/migrations"
)

type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) InTx(ctx context.Context, fn func(store.Queries) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return fn(&queries{db: tx}) })
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }
func (s *Store) Close()                         { s.pool.Close() }

// Migrate applies pending migrations and returns how many ran.
func Migrate(ctx context.Context, url string) (applied int, err error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close migration connection: %w", cerr)
		}
	}()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return 0, fmt.Errorf("load migrations: %w", err)
	}
	results, err := p.Up(ctx)
	return len(results), err
}

type dbtx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type queries struct{ db dbtx }

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func mapErr(err error, what string) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows), pgCode(err) == "23503": // foreign_key_violation
		return store.NotFound(what)
	case pgCode(err) == "23505": // unique_violation
		return domain.Errorf(domain.CodeConflict, "%s already exists", what)
	}
	return err
}

// ---- tenants and quotas

const tenantCols = `id, slug, name, tier, created_at`

func scanTenant(row pgx.Row) (domain.Tenant, error) {
	var t domain.Tenant
	var tier string
	err := row.Scan(&t.ID, &t.Slug, &t.Name, &tier, &t.CreatedAt)
	t.Tier = domain.IsolationTier(tier)
	return t, err
}

func (q *queries) CreateTenant(ctx context.Context, t domain.Tenant) error {
	_, err := q.db.Exec(ctx, `INSERT INTO tenants (`+tenantCols+`) VALUES ($1, $2, $3, $4, $5)`,
		t.ID, t.Slug, t.Name, string(t.Tier), t.CreatedAt)
	return mapErr(err, "tenant")
}

func (q *queries) TenantBySlug(ctx context.Context, slug string) (domain.Tenant, error) {
	t, err := scanTenant(q.db.QueryRow(ctx, `SELECT `+tenantCols+` FROM tenants WHERE slug = $1`, slug))
	return t, mapErr(err, "tenant")
}

func (q *queries) TenantByID(ctx context.Context, id uuid.UUID) (domain.Tenant, error) {
	t, err := scanTenant(q.db.QueryRow(ctx, `SELECT `+tenantCols+` FROM tenants WHERE id = $1`, id))
	return t, mapErr(err, "tenant")
}

func (q *queries) SetQuota(ctx context.Context, quota domain.Quota) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO quotas (tenant_id, gpu_class, max_gpus) VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, gpu_class) DO UPDATE SET max_gpus = EXCLUDED.max_gpus`,
		quota.TenantID, quota.GPUClass, quota.MaxGPUs)
	return mapErr(err, "tenant")
}

func (q *queries) Quotas(ctx context.Context, tenantID uuid.UUID) ([]domain.Quota, error) {
	rows, err := q.db.Query(ctx,
		`SELECT tenant_id, gpu_class, max_gpus FROM quotas WHERE tenant_id = $1 ORDER BY gpu_class`, tenantID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Quota, error) {
		var x domain.Quota
		err := r.Scan(&x.TenantID, &x.GPUClass, &x.MaxGPUs)
		return x, err
	})
}

func (q *queries) LockQuota(ctx context.Context, tenantID uuid.UUID, gpuClass string) (int, error) {
	var limit int
	err := q.db.QueryRow(ctx,
		`SELECT max_gpus FROM quotas WHERE tenant_id = $1 AND gpu_class = $2 FOR UPDATE`,
		tenantID, gpuClass).Scan(&limit)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return limit, err
}

func (q *queries) GPUsHeld(ctx context.Context, tenantID uuid.UUID, gpuClass string) (int, error) {
	var n int
	err := q.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(gpu_count), 0) FROM workloads
		WHERE tenant_id = $1 AND gpu_class = $2 AND state = ANY($3)`,
		tenantID, gpuClass, store.StatesAsStrings(domain.StatesWhere(domain.State.HoldsQuota))).Scan(&n)
	return n, err
}

// ---- workloads

const workloadCols = `id, tenant_id, name, kind, state, image_id, gpu_class, gpu_count, params, created_by, created_at, updated_at`

func scanWorkload(row pgx.Row) (domain.Workload, error) {
	var w domain.Workload
	var kind, state string
	err := row.Scan(&w.ID, &w.TenantID, &w.Name, &kind, &state, &w.ImageID, &w.GPUClass,
		&w.GPUCount, &w.Params, &w.CreatedBy, &w.CreatedAt, &w.UpdatedAt)
	w.Kind, w.State = domain.WorkloadKind(kind), domain.State(state)
	return w, err
}

func (q *queries) collectWorkloads(ctx context.Context, sql string, args ...any) ([]domain.Workload, error) {
	rows, err := q.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Workload, error) { return scanWorkload(r) })
}

func (q *queries) CreateWorkload(ctx context.Context, w domain.Workload) error {
	params := w.Params
	if params == nil {
		params = map[string]string{}
	}
	_, err := q.db.Exec(ctx, `INSERT INTO workloads (`+workloadCols+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		w.ID, w.TenantID, w.Name, string(w.Kind), string(w.State), w.ImageID, w.GPUClass,
		w.GPUCount, params, w.CreatedBy, w.CreatedAt, w.UpdatedAt)
	return mapErr(err, "workload")
}

func (q *queries) GetWorkload(ctx context.Context, tenantID, id uuid.UUID) (domain.Workload, error) {
	w, err := scanWorkload(q.db.QueryRow(ctx, `SELECT `+workloadCols+` FROM workloads
		WHERE id = $1 AND tenant_id = $2 AND state <> 'deleted'`, id, tenantID))
	return w, mapErr(err, "workload")
}

func (q *queries) GetWorkloadHistory(ctx context.Context, tenantID, id uuid.UUID) (domain.Workload, error) {
	w, err := scanWorkload(q.db.QueryRow(ctx, `SELECT `+workloadCols+` FROM workloads
		WHERE id = $1 AND tenant_id = $2`, id, tenantID))
	return w, mapErr(err, "workload")
}

func (q *queries) ListWorkloads(ctx context.Context, tenantID uuid.UUID) ([]domain.Workload, error) {
	return q.collectWorkloads(ctx, `SELECT `+workloadCols+` FROM workloads
		WHERE tenant_id = $1 AND state <> 'deleted' ORDER BY created_at, id`, tenantID)
}

func (q *queries) ListWorkloadHistory(ctx context.Context, tenantID uuid.UUID) ([]domain.Workload, error) {
	return q.collectWorkloads(ctx, `SELECT `+workloadCols+` FROM workloads
		WHERE tenant_id = $1 ORDER BY created_at, id`, tenantID)
}

func (q *queries) UpdateWorkloadState(ctx context.Context, id uuid.UUID, from, to domain.State, at time.Time) error {
	tag, err := q.db.Exec(ctx, `UPDATE workloads SET state = $3, updated_at = $4 WHERE id = $1 AND state = $2`,
		id, string(from), string(to), at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.StaleState(id, from)
	}
	return nil
}

func (q *queries) WorkloadsInStates(ctx context.Context, states ...domain.State) ([]domain.Workload, error) {
	return q.collectWorkloads(ctx, `SELECT `+workloadCols+` FROM workloads
		WHERE state = ANY($1) ORDER BY created_at, id`, store.StatesAsStrings(states))
}

func (q *queries) GPUsAllocated(ctx context.Context) (map[string]int, error) {
	rows, err := q.db.Query(ctx, `SELECT gpu_class, SUM(gpu_count) FROM workloads
		WHERE state = ANY($1) GROUP BY gpu_class`,
		store.StatesAsStrings(domain.StatesWhere(domain.State.HoldsCapacity)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var class string
		var n int
		if err := rows.Scan(&class, &n); err != nil {
			return nil, err
		}
		out[class] = n
	}
	return out, rows.Err()
}

func (q *queries) WorkloadsForMetering(ctx context.Context, since time.Time) ([]domain.Workload, error) {
	return q.collectWorkloads(ctx, `SELECT `+workloadCols+` FROM workloads w
		WHERE state = ANY($1)
		   OR EXISTS (SELECT 1 FROM workload_events e WHERE e.workload_id = w.id AND e.at >= $2)
		ORDER BY created_at, id`,
		store.StatesAsStrings(domain.StatesWhere(domain.State.Metered)), since)
}

// ---- events and usage

func (q *queries) AppendEvent(ctx context.Context, e domain.Event) error {
	_, err := q.db.Exec(ctx, `INSERT INTO workload_events (workload_id, from_state, to_state, reason, at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5)`,
		e.WorkloadID, string(e.From), string(e.To), e.Reason, e.At)
	return mapErr(err, "workload")
}

func (q *queries) Events(ctx context.Context, workloadID uuid.UUID) ([]domain.Event, error) {
	return q.collectEvents(ctx, `SELECT id, workload_id, COALESCE(from_state, ''), to_state, reason, at
		FROM workload_events WHERE workload_id = $1 ORDER BY at, id`, workloadID)
}

func (q *queries) EventsForWorkload(ctx context.Context, tenantID, workloadID uuid.UUID) ([]domain.Event, error) {
	return q.collectEvents(ctx, `SELECT e.id, e.workload_id, COALESCE(e.from_state, ''), e.to_state, e.reason, e.at
		FROM workload_events e JOIN workloads w ON w.id = e.workload_id AND w.tenant_id = $2
		WHERE e.workload_id = $1 ORDER BY e.at, e.id`, workloadID, tenantID)
}

func (q *queries) collectEvents(ctx context.Context, sql string, args ...any) ([]domain.Event, error) {
	rows, err := q.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Event, error) {
		var e domain.Event
		var from, to string
		err := r.Scan(&e.ID, &e.WorkloadID, &from, &to, &e.Reason, &e.At)
		e.From, e.To = domain.State(from), domain.State(to)
		return e, err
	})
}

func (q *queries) InsertUsageIfAbsent(ctx context.Context, r domain.UsageRecord) (bool, error) {
	tag, err := q.db.Exec(ctx, `INSERT INTO usage_records
		(workload_id, period_start, tenant_id, gpu_class, gpu_count, seconds)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (workload_id, period_start) DO NOTHING`,
		r.WorkloadID, r.PeriodStart, r.TenantID, r.GPUClass, r.GPUCount, r.Seconds)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

const usageCols = `workload_id, period_start, tenant_id, gpu_class, gpu_count, seconds`

func (q *queries) collectUsage(ctx context.Context, sql string, args ...any) ([]domain.UsageRecord, error) {
	rows, err := q.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.UsageRecord, error) {
		var u domain.UsageRecord
		err := r.Scan(&u.WorkloadID, &u.PeriodStart, &u.TenantID, &u.GPUClass, &u.GPUCount, &u.Seconds)
		return u, err
	})
}

func (q *queries) Usage(ctx context.Context, tenantID uuid.UUID, from, to time.Time) ([]domain.UsageRecord, error) {
	return q.collectUsage(ctx, `SELECT `+usageCols+` FROM usage_records
		WHERE tenant_id = $1 AND period_start >= $2 AND period_start < $3
		ORDER BY period_start, workload_id`, tenantID, from, to)
}

func (q *queries) UsageForWorkload(ctx context.Context, tenantID, workloadID uuid.UUID) ([]domain.UsageRecord, error) {
	return q.collectUsage(ctx, `SELECT `+usageCols+` FROM usage_records
		WHERE tenant_id = $1 AND workload_id = $2 ORDER BY period_start`, tenantID, workloadID)
}
