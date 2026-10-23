// Package devseed creates the tenants used by `tensora dev` and by tests.
package devseed

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/store"
)

type seedTenant struct {
	slug, name string
	tier       domain.IsolationTier
	quotas     map[string]int
}

var tenants = []seedTenant{
	{"acme", "Acme Robotics (invited team)", domain.TierUntrusted, map[string]int{"l40s-48gb": 2, "a100-80gb": 1}},
	{"globex", "Globex (community users)", domain.TierUntrusted, map[string]int{"l40s-48gb": 1}},
	{"internal", "Tensora internal", domain.TierTrusted, map[string]int{"l40s-48gb": 4, "a100-80gb": 2, "rtx4090-24gb": 2}},
}

// Seed is idempotent: existing tenants are left alone.
func Seed(ctx context.Context, st store.Store, now time.Time) error {
	return st.InTx(ctx, func(q store.Queries) error {
		for _, s := range tenants {
			if _, err := q.TenantBySlug(ctx, s.slug); err == nil {
				continue
			} else if !domain.IsNotFound(err) {
				return err
			}
			t := domain.Tenant{ID: uuid.Must(uuid.NewV7()), Slug: s.slug, Name: s.name, Tier: s.tier, CreatedAt: now.UTC()}
			if err := q.CreateTenant(ctx, t); err != nil {
				return err
			}
			for class, n := range s.quotas {
				if err := q.SetQuota(ctx, domain.Quota{TenantID: t.ID, GPUClass: class, MaxGPUs: n}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
