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

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/catalog"
	"github.com/tensora/tensora/controlplane/internal/cluster"
	"github.com/tensora/tensora/controlplane/internal/cluster/manifests"
	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/service"
	"github.com/tensora/tensora/controlplane/internal/store"
)

// errNotInCatalog marks a workload whose GPU class or image was removed from the catalog.
// Retrying can't fix it; the message is safe to show the owner.
var errNotInCatalog = errors.New("is no longer offered by this installation")

type Options struct {
	// Region is the one region this controller reconciles (ADR-0009). It never sees, counts, or
	// admits another region's workloads. Empty means local, the region of a Solo install.
	Region                domain.Region
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
	if opts.Region == "" {
		opts.Region = domain.RegionLocal
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
			c.log.Error("reconcile pass failed", "err", service.RedactSecrets(err.Error()))
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
		if waiting, err = q.WorkloadsInStates(ctx, c.opts.Region, domain.StatePending, domain.StateQueued); err != nil {
			return err
		}
		allocated, err = q.GPUsAllocated(ctx, c.opts.Region)
		return err
	})
	if err != nil || len(waiting) == 0 {
		return wrap("admit", err)
	}
	capacity, err := c.backend.Capacity(ctx)
	if err != nil {
		for _, w := range waiting {
			c.note(ctx, w, "couldn't check for free GPUs on the runtime; retrying. The operator's log has the details.")
		}
		return wrap("admit: capacity", err)
	}
	// FIFO with backfill: a large request at the head does not block smaller ones behind it.
	// TODO(phase-3): add aging so large requests cannot starve.
	for _, w := range waiting {
		// Checked before capacity: a class that left the catalog has no capacity either, so the
		// workload would otherwise wait forever under a misleading reason. The launch rules are
		// checked again too, in case the catalog or the tenant's tier changed since create.
		runtime, err := c.checkLaunch(ctx, w)
		if errors.Is(err, errRetryLater) {
			continue
		}
		if err != nil {
			c.transition(ctx, w, domain.StateFailed, "can't start: "+userMessage(err))
			continue
		}
		free := max(capacity[w.GPUClass]-allocated[w.GPUClass], 0)
		c.log.Debug("admission check", "workload", w.ID, "gpu_class", w.GPUClass,
			"requested", w.GPUCount, "capacity", capacity[w.GPUClass], "allocated", allocated[w.GPUClass])
		if free < w.GPUCount {
			if w.State == domain.StatePending {
				c.transition(ctx, w, domain.StateQueued, c.waitingReason(ctx, w, free))
			}
			continue
		}
		if err := c.provision(ctx, w); err != nil {
			c.retryLater(ctx, w, "start it on the runtime", err)
			continue
		}
		// Placed on the requested class. Fallback placement (ADR-0010) and agent machines will choose
		// here, in one place, as admission grows (ADR-0014's sharing rules too).
		placed, machine := w.GPUClass, uuid.Nil
		// Counted even if placing fails: its objects were just applied, and the next pass retries.
		allocated[placed] += w.GPUCount
		c.place(ctx, w, placed, machine, runtime.Server)
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
		var obs cluster.Observed
		var reason string
		if err == nil {
			obs, reason, err = c.backend.Observe(ctx, ns, w.ID, w.Kind)
		}
		if err != nil {
			// A status that can't be read must not hold GPUs forever: the provisioning timeout
			// still applies.
			if w.State == domain.StateProvisioning && c.now().Sub(w.UpdatedAt) > c.opts.ProvisionTimeout {
				c.logError("observe failed", w, err)
				c.fail(ctx, w, fmt.Sprintf("not ready after %s: couldn't read its status from the runtime", c.opts.ProvisionTimeout))
				continue
			}
			c.retryLater(ctx, w, "read its status from the runtime", err)
			continue
		}
		if w.PlacedClass == "" {
			// Admitted by a version that didn't record placement. Fallbacks didn't exist then, so it
			// runs on the class it requested; record that, so it can be counted and accounted.
			if !c.adoptPlacement(ctx, &w) {
				continue
			}
		}
		switch {
		case obs == cluster.ObservedFailed:
			c.fail(ctx, w, "workload failed: "+reason)
			continue
		case w.State == domain.StateRunning && obs == cluster.ObservedAbsent:
			c.fail(ctx, w, "cluster objects disappeared")
			continue
		case w.State == domain.StateProvisioning && obs == cluster.ObservedReady:
			if c.transition(ctx, w, domain.StateRunning, "ready") {
				continue
			}
		}
		// Still provisioning, including when the move to running didn't happen: a workload must
		// never hold GPUs forever.
		if w.State == domain.StateProvisioning && c.now().Sub(w.UpdatedAt) > c.opts.ProvisionTimeout {
			c.fail(ctx, w, fmt.Sprintf("not ready after %s: %s", c.opts.ProvisionTimeout, reason))
		}
	}
	return nil
}

// adoptPlacement records the requested class as the placement of a workload that holds capacity
// without one, and says so in its timeline.
func (c *Controller) adoptPlacement(ctx context.Context, w *domain.Workload) bool {
	err := c.store.InTx(ctx, func(q store.Queries) error {
		return q.SetPlacement(ctx, w.ID, w.GPUClass, w.MachineID)
	})
	if err != nil {
		c.logError("adopt placement failed", *w, err)
		return false
	}
	w.PlacedClass = w.GPUClass
	c.note(ctx, *w, fmt.Sprintf("placed on %s: it was admitted before placements were recorded", w.GPUClass))
	return true
}

// park frees the GPUs of stopping workloads while keeping their disks.
func (c *Controller) park(ctx context.Context) error {
	ws, err := c.inStates(ctx, domain.StateStopping)
	if err != nil {
		return wrap("park", err)
	}
	for _, w := range ws {
		if err := c.release(ctx, w); err != nil {
			c.retryLater(ctx, w, "stop it on the runtime", err)
			continue
		}
		ns, _ := c.namespace(ctx, w)
		obs, _, err := c.backend.Observe(ctx, ns, w.ID, w.Kind)
		if err != nil {
			c.retryLater(ctx, w, "read its status from the runtime", err)
			continue
		}
		if obs == cluster.ObservedStopped || obs == cluster.ObservedAbsent {
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
			c.retryLater(ctx, w, "delete it from the runtime", err)
			continue
		}
		obs, _, err := c.backend.Observe(ctx, ns, w.ID, w.Kind)
		if err != nil {
			c.retryLater(ctx, w, "read its status from the runtime", err)
			continue
		}
		if obs == cluster.ObservedAbsent {
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
		// The failure's reason is recorded with the Failed transition. Leaving it out of the note
		// keeps one note per problem, even when the runtime's status keeps changing.
		c.retryLater(ctx, w, "release its GPUs on the runtime after it failed", err)
		return
	}
	c.transition(ctx, w, domain.StateFailed, reason)
}

// waitingReason says why a workload waits for capacity. Only trusted tenants (a machine's owner
// and their team) see how many GPUs are free. For invited and community users that number would
// show when other people use the machines (docs/architecture/tenancy-and-isolation.md).
func (c *Controller) waitingReason(ctx context.Context, w domain.Workload, free int) string {
	units := "GPU(s)"
	if g, ok := c.catalog.GPUClass(w.GPUClass); ok && g.Accelerator == domain.AcceleratorCPU {
		units = "unit(s)"
	}
	if t, err := c.tenant(ctx, w); err == nil && t.Tier == domain.TierTrusted {
		return fmt.Sprintf("waiting for %s capacity: requested %d %s, %d free", w.GPUClass, w.GPUCount, units, free)
	}
	return fmt.Sprintf("waiting for %s capacity: requested %d %s, not enough free right now", w.GPUClass, w.GPUCount, units)
}

// retryLater handles a runtime action that failed and will be retried next pass. The operator's
// log gets the error; the owner's timeline gets a note saying so, never the raw error, which can
// name internal objects or echo inputs.
func (c *Controller) retryLater(ctx context.Context, w domain.Workload, action string, err error) {
	c.logError("couldn't "+action, w, err)
	if errors.Is(err, errNotInCatalog) {
		c.note(ctx, w, fmt.Sprintf("couldn't %s: %v. Retrying won't help until the operator restores it.", action, err))
		return
	}
	c.note(ctx, w, fmt.Sprintf("couldn't %s; retrying. The operator's log has the details under this workload's ID.", action))
}

// note adds a timeline entry without a state change, unless the workload has moved on or its
// latest entry already says the same, so a retry loop adds one entry rather than one per pass.
func (c *Controller) note(ctx context.Context, w domain.Workload, reason string) {
	reason = service.CleanReason(reason)
	err := c.store.InTx(ctx, func(q store.Queries) error {
		current, err := q.GetWorkloadHistory(ctx, w.TenantID, w.ID)
		if err != nil || current.State != w.State {
			return err
		}
		events, err := q.Events(ctx, w.ID)
		if err != nil {
			return err
		}
		if n := len(events); n > 0 && events[n-1].Reason == reason {
			return nil
		}
		return service.Note(ctx, q, w, reason, c.now())
	})
	if err != nil {
		c.log.Warn("note skipped", "workload", w.ID, "err", err)
	}
}

// logError writes a failure to the operator's log. Runtime errors can echo inputs such as a model
// URI, so credentials are redacted first.
func (c *Controller) logError(msg string, w domain.Workload, err error) {
	c.log.Error(msg, "workload", w.ID, "tenant", w.TenantID, "err", service.RedactSecrets(err.Error()))
}

// place records where the workload was placed and moves it to provisioning, in one transaction,
// so a workload is never provisioning without a placement. The reason names the model server for
// endpoints, so a change of server is never silent (rule 10).
func (c *Controller) place(ctx context.Context, w domain.Workload, placedClass string, machineID uuid.UUID, server string) bool {
	reason := fmt.Sprintf("admitted: %d × %s", w.GPUCount, placedClass)
	if server != "" {
		reason += ", served by " + server
	}
	return c.transitionWith(ctx, w, domain.StateProvisioning, reason,
		func(q store.Queries, w *domain.Workload) error {
			if err := q.SetPlacement(ctx, w.ID, placedClass, machineID); err != nil {
				return err
			}
			w.PlacedClass, w.MachineID = placedClass, machineID
			return nil
		})
}

func (c *Controller) transition(ctx context.Context, w domain.Workload, to domain.State, reason string) bool {
	return c.transitionWith(ctx, w, to, reason, nil)
}

// transitionWith runs before (if any) and the transition in one transaction.
func (c *Controller) transitionWith(ctx context.Context, w domain.Workload, to domain.State, reason string,
	before func(store.Queries, *domain.Workload) error,
) bool {
	reason = service.CleanReason(reason)
	from := w.State
	err := c.store.InTx(ctx, func(q store.Queries) error {
		if before != nil {
			if err := before(q, &w); err != nil {
				return err
			}
		}
		return service.Transition(ctx, q, &w, to, reason, c.now())
	})
	if err != nil {
		c.log.Warn("transition skipped", "workload", w.ID, "from", from, "to", to, "err", service.RedactSecrets(err.Error()))
		// A conflict is a concurrent change, such as a delete during provisioning; the next pass
		// sees the new state, and the timeline already shows it. Anything else is a decision that
		// didn't happen, so the owner hears about it (rule 14).
		if code, _ := domain.CodeOf(err); code != domain.CodeConflict {
			c.note(ctx, w, fmt.Sprintf("couldn't move it to %s; retrying. The operator's log has the details under this workload's ID.", to))
		}
		return false
	}
	c.log.Info("workload transitioned", "workload", w.ID, "tenant", w.TenantID, "from", from, "to", to, "reason", reason)
	return true
}

func (c *Controller) inStates(ctx context.Context, states ...domain.State) ([]domain.Workload, error) {
	var ws []domain.Workload
	err := c.store.InTx(ctx, func(q store.Queries) error {
		var err error
		ws, err = q.WorkloadsInStates(ctx, c.opts.Region, states...)
		return err
	})
	return ws, err
}

func (c *Controller) tenant(ctx context.Context, w domain.Workload) (domain.Tenant, error) {
	var t domain.Tenant
	err := c.store.InTx(ctx, func(q store.Queries) error {
		var err error
		t, err = q.TenantByID(ctx, w.TenantID)
		return err
	})
	return t, err
}

func (c *Controller) namespace(ctx context.Context, w domain.Workload) (string, error) {
	t, err := c.tenant(ctx, w)
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
	gpu, img, runtime, err := c.catalogEntries(w)
	if err != nil {
		return manifests.Context{}, nil, err
	}
	return manifests.Context{
		TenantSlug:            t.Slug,
		Tier:                  t.Tier,
		GPU:                   gpu,
		Image:                 img,
		Runtime:               runtime,
		StorageClass:          c.opts.StorageClass,
		ImagesNamespace:       c.opts.ImagesNamespace,
		UntrustedRuntimeClass: c.opts.UntrustedRuntimeClass,
	}, quotas, nil
}

// errRetryLater means a check couldn't be made this pass; nothing is decided.
var errRetryLater = errors.New("retry later")

// checkLaunch re-applies catalog.CheckLaunch at admission, with the tenant's current tier.
func (c *Controller) checkLaunch(ctx context.Context, w domain.Workload) (catalog.Runtime, error) {
	gpu, img, _, err := c.catalogEntries(w)
	if err != nil {
		return catalog.Runtime{}, err
	}
	t, err := c.tenant(ctx, w)
	if err != nil {
		c.logError("admission: tenant lookup failed", w, err)
		return catalog.Runtime{}, errRetryLater
	}
	return catalog.CheckLaunch(t.Tier, gpu, img, w.Kind, w.Region)
}

// userMessage is the part of an error that's safe to show the workload's owner: a domain error's
// message, or the error itself for the controller's own fixed-vocabulary errors.
func userMessage(err error) string {
	var de *domain.Error
	if errors.As(err, &de) {
		return de.Message
	}
	return err.Error()
}

// catalogEntries looks up the workload's class, its image, and the image's runtime for that
// class's accelerator. An operator may have removed any of them since the workload was created.
func (c *Controller) catalogEntries(w domain.Workload) (catalog.GPUClass, catalog.Image, catalog.Runtime, error) {
	gpu, ok := c.catalog.GPUClass(w.GPUClass)
	if !ok {
		return catalog.GPUClass{}, catalog.Image{}, catalog.Runtime{}, fmt.Errorf("GPU class %q %w", w.GPUClass, errNotInCatalog)
	}
	img, ok := c.catalog.Image(w.ImageID)
	if !ok {
		return catalog.GPUClass{}, catalog.Image{}, catalog.Runtime{}, fmt.Errorf("image %q %w", w.ImageID, errNotInCatalog)
	}
	runtime, ok := img.RuntimeFor(gpu.Accelerator)
	if !ok {
		return catalog.GPUClass{}, catalog.Image{}, catalog.Runtime{}, fmt.Errorf("image %q on %s %w", w.ImageID, gpu.Accelerator, errNotInCatalog)
	}
	return gpu, img, runtime, nil
}

// quotaResources turns per-class quotas into ResourceQuota limits on the extended resources
// those classes consume. Classes sharing nvidia.com/gpu are summed (it's a backstop only).
func (c *Controller) quotaResources(quotas []domain.Quota) map[string]int {
	out := map[string]int{}
	for _, q := range quotas {
		gc, ok := c.catalog.GPUClass(q.GPUClass)
		if !ok || gc.ContainerResource == "" { // Apple and CPU classes don't run on Kubernetes
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
