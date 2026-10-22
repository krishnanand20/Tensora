// Package controller reconciles workload intent in Postgres with objects in Kubernetes.
// It is the only component with cluster credentials. Each pass is idempotent: cluster
// writes are server-side applies, and state changes are compare-and-swap transitions.
package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/tensora/tensora/controlplane/internal/catalog"
	"github.com/tensora/tensora/controlplane/internal/cluster"
	"github.com/tensora/tensora/controlplane/internal/cluster/manifests"
	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/service"
	"github.com/tensora/tensora/controlplane/internal/store"
)

type Options struct {
	StorageClass          string
	ImagesNamespace       string
	UntrustedRuntimeClass string
	ProvisionTimeout      time.Duration
}

type Controller struct {
	store   store.Store
	backend cluster.Backend
	catalog *catalog.Catalog
	opts    Options
	log     *slog.Logger
	now     func() time.Time
}

func New(st store.Store, b cluster.Backend, cat *catalog.Catalog, opts Options, log *slog.Logger, now func() time.Time) *Controller {
	if now == nil {
		now = time.Now
	}
	return &Controller{store: st, backend: b, catalog: cat, opts: opts, log: log, now: now}
}

// Run reconciles every interval until ctx is cancelled.
func (c *Controller) Run(ctx context.Context, interval time.Duration) error {
	c.log.Info("controller started", "interval", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := c.ReconcileOnce(ctx); err != nil {
			c.log.Error("reconcile pass failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// ReconcileOnce runs one pass. Per-workload failures are logged and retried next pass;
// only failures that stop a whole step are returned.
func (c *Controller) ReconcileOnce(ctx context.Context) error {
	return errors.Join(c.admit(ctx), c.observe(ctx), c.park(ctx), c.remove(ctx))
}

// admit moves pending/queued workloads onto free GPUs. Capacity comes from the cluster;
// allocation comes from Postgres, because every GPU workload goes through Tensora.
func (c *Controller) admit(ctx context.Context) error {
	var waiting []domain.Workload
	var allocated map[string]int
	err := c.store.InTx(ctx, func(q store.Queries) error {
		var err error
		if waiting, err = q.WorkloadsInStates(ctx, domain.StatePending, domain.StateQueued); err != nil {
			return err
		}
		allocated, err = q.GPUsAllocated(ctx)
		return err
	})
	if err != nil || len(waiting) == 0 {
		return wrap("admit", err)
	}
	capacity, err := c.backend.Capacity(ctx)
	if err != nil {
		return wrap("admit: capacity", err)
	}
	// FIFO with backfill: a large request at the head does not block smaller ones behind it.
	// TODO(phase-3): add aging so large requests cannot starve.
	for _, w := range waiting {
		if capacity[w.GPUClass]-allocated[w.GPUClass] < w.GPUCount {
			if w.State == domain.StatePending {
				c.transition(ctx, w, domain.StateQueued, "waiting for GPU capacity")
			}
			continue
		}
		if err := c.provision(ctx, w); err != nil {
			c.log.Error("provision failed", "workload", w.ID, "err", err)
			continue
		}
		if c.transition(ctx, w, domain.StateProvisioning, "admitted") {
			allocated[w.GPUClass] += w.GPUCount
		}
	}
	return nil
}

// observe promotes ready workloads to running and fails broken or stuck ones.
func (c *Controller) observe(ctx context.Context) error {
	ws, err := c.inStates(ctx, domain.StateProvisioning, domain.StateRunning)
	if err != nil {
		return wrap("observe", err)
	}
	for _, w := range ws {
		ns, err := c.namespace(ctx, w)
		if err != nil {
			c.log.Error("observe", "workload", w.ID, "err", err)
			continue
		}
		obs, reason, err := c.backend.Observe(ctx, ns, w.ID, w.Kind)
		if err != nil {
			c.log.Error("observe", "workload", w.ID, "err", err)
			continue
		}
		switch {
		case obs == cluster.ObservedFailed:
			c.fail(ctx, w, "workload failed: "+reason)
		case w.State == domain.StateProvisioning && obs == cluster.ObservedReady:
			c.transition(ctx, w, domain.StateRunning, "ready")
		case w.State == domain.StateRunning && obs == cluster.ObservedAbsent:
			c.fail(ctx, w, "cluster objects disappeared")
		case w.State == domain.StateProvisioning && c.now().Sub(w.UpdatedAt) > c.opts.ProvisionTimeout:
			c.fail(ctx, w, fmt.Sprintf("not ready after %s: %s", c.opts.ProvisionTimeout, reason))
		}
	}
	return nil
}

// park frees the GPUs of stopping workloads while keeping their disks.
func (c *Controller) park(ctx context.Context) error {
	ws, err := c.inStates(ctx, domain.StateStopping)
	if err != nil {
		return wrap("park", err)
	}
	for _, w := range ws {
		if err := c.release(ctx, w); err != nil {
			c.log.Error("park", "workload", w.ID, "err", err)
			continue
		}
		ns, _ := c.namespace(ctx, w)
		obs, _, err := c.backend.Observe(ctx, ns, w.ID, w.Kind)
		if err == nil && (obs == cluster.ObservedStopped || obs == cluster.ObservedAbsent) {
			c.transition(ctx, w, domain.StateStopped, "stopped")
		}
	}
	return nil
}

// remove deletes cluster objects (including volumes) for deleting workloads.
func (c *Controller) remove(ctx context.Context) error {
	ws, err := c.inStates(ctx, domain.StateDeleting)
	if err != nil {
		return wrap("remove", err)
	}
	for _, w := range ws {
		ns, err := c.namespace(ctx, w)
		if err == nil {
			err = c.backend.Delete(ctx, ns, w.ID)
		}
		if err != nil {
			c.log.Error("remove", "workload", w.ID, "err", err)
			continue
		}
		obs, _, err := c.backend.Observe(ctx, ns, w.ID, w.Kind)
		if err == nil && obs == cluster.ObservedAbsent {
			c.transition(ctx, w, domain.StateDeleted, "deleted")
		}
	}
	return nil
}

func (c *Controller) provision(ctx context.Context, w domain.Workload) error {
	bc, quotas, err := c.buildContext(ctx, w)
	if err != nil {
		return err
	}
	objs, err := manifests.Workload(w, bc, true)
	if err != nil {
		return err
	}
	tenant := manifests.Tenant(bc.TenantSlug, c.quotaResources(quotas), c.opts.ImagesNamespace)
	return c.backend.Apply(ctx, append(tenant, objs...))
}

// release parks a workload's GPUs: VMs halt and notebooks scale to zero (disks kept);
// endpoints are deleted because they have nothing worth keeping.
func (c *Controller) release(ctx context.Context, w domain.Workload) error {
	if w.Kind == domain.KindEndpoint {
		ns, err := c.namespace(ctx, w)
		if err != nil {
			return err
		}
		return c.backend.Delete(ctx, ns, w.ID)
	}
	bc, _, err := c.buildContext(ctx, w)
	if err != nil {
		return err
	}
	objs, err := manifests.Workload(w, bc, false)
	if err != nil {
		return err
	}
	return c.backend.Apply(ctx, objs)
}

// fail releases GPUs before marking the workload failed, so failed workloads never
// hold capacity the scheduler believes is free.
func (c *Controller) fail(ctx context.Context, w domain.Workload, reason string) {
	if err := c.release(ctx, w); err != nil {
		c.log.Error("release before fail", "workload", w.ID, "err", err)
		return
	}
	c.transition(ctx, w, domain.StateFailed, reason)
}

func (c *Controller) transition(ctx context.Context, w domain.Workload, to domain.State, reason string) bool {
	from := w.State
	err := c.store.InTx(ctx, func(q store.Queries) error {
		return service.Transition(ctx, q, &w, to, reason, c.now())
	})
	if err != nil {
		// Usually a concurrent API change (e.g. delete during provisioning); next pass sees it.
		c.log.Warn("transition skipped", "workload", w.ID, "from", from, "to", to, "err", err)
		return false
	}
	c.log.Info("workload transitioned", "workload", w.ID, "tenant", w.TenantID, "from", from, "to", to, "reason", reason)
	return true
}

func (c *Controller) inStates(ctx context.Context, states ...domain.State) ([]domain.Workload, error) {
	var ws []domain.Workload
	err := c.store.InTx(ctx, func(q store.Queries) error {
		var err error
		ws, err = q.WorkloadsInStates(ctx, states...)
		return err
	})
	return ws, err
}

func (c *Controller) namespace(ctx context.Context, w domain.Workload) (string, error) {
	var t domain.Tenant
	err := c.store.InTx(ctx, func(q store.Queries) error {
		var err error
		t, err = q.TenantByID(ctx, w.TenantID)
		return err
	})
	return cluster.TenantNamespace(t.Slug), err
}

func (c *Controller) buildContext(ctx context.Context, w domain.Workload) (manifests.Context, []domain.Quota, error) {
	var t domain.Tenant
	var quotas []domain.Quota
	err := c.store.InTx(ctx, func(q store.Queries) error {
		var err error
		if t, err = q.TenantByID(ctx, w.TenantID); err != nil {
			return err
		}
		quotas, err = q.Quotas(ctx, w.TenantID)
		return err
	})
	if err != nil {
		return manifests.Context{}, nil, err
	}
	gpu, ok := c.catalog.GPUClass(w.GPUClass)
	if !ok {
		return manifests.Context{}, nil, fmt.Errorf("gpu class %q is no longer in the catalog", w.GPUClass)
	}
	img, ok := c.catalog.Image(w.ImageID)
	if !ok {
		return manifests.Context{}, nil, fmt.Errorf("image %q is no longer in the catalog", w.ImageID)
	}
	return manifests.Context{
		TenantSlug:            t.Slug,
		Tier:                  t.Tier,
		GPU:                   gpu,
		Image:                 img,
		StorageClass:          c.opts.StorageClass,
		ImagesNamespace:       c.opts.ImagesNamespace,
		UntrustedRuntimeClass: c.opts.UntrustedRuntimeClass,
	}, quotas, nil
}

// quotaResources turns per-class quotas into ResourceQuota limits on the extended resources
// those classes consume. Classes sharing nvidia.com/gpu are summed (it's a backstop only).
func (c *Controller) quotaResources(quotas []domain.Quota) map[string]int {
	out := map[string]int{}
	for _, q := range quotas {
		gc, ok := c.catalog.GPUClass(q.GPUClass)
		if !ok {
			continue
		}
		out[gc.ContainerResource] += q.MaxGPUs
		if gc.VMDeviceName != "" {
			out[gc.VMDeviceName] += q.MaxGPUs
		}
	}
	return out
}

func wrap(step string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", step, err)
}
