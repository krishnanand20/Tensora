package controller_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/cluster"
	"github.com/tensora/tensora/controlplane/internal/cluster/fake"
	"github.com/tensora/tensora/controlplane/internal/controller"
	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/service"
	"github.com/tensora/tensora/controlplane/internal/testutil"
)

type harness struct {
	t       *testing.T
	svc     *service.Service
	ctrl    *controller.Controller
	backend *fake.Backend
	clock   *testutil.Clock
}

func newHarness(t *testing.T, capacity map[string]int) *harness {
	st := testutil.SeededStore(t)
	cat := testutil.Catalog(t)
	clock := testutil.NewClock(testutil.T0)
	b := fake.New(capacity)
	ctrl := controller.New(st, b, cat, controller.Options{
		StorageClass: "tensora-block", ImagesNamespace: "tensora-images",
		UntrustedRuntimeClass: "kata", ProvisionTimeout: 10 * time.Minute,
	}, slog.New(slog.DiscardHandler), clock.Now)
	return &harness{t: t, svc: service.New(st, cat, clock.Now), ctrl: ctrl, backend: b, clock: clock}
}

func (h *harness) reconcile() {
	h.t.Helper()
	if err := h.ctrl.ReconcileOnce(context.Background()); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) create(p domain.Principal, name string, gpus int) domain.Workload {
	h.t.Helper()
	w, err := h.svc.CreateWorkload(context.Background(), p, service.CreateWorkloadInput{
		Name: name, Kind: domain.KindNotebook, ImageID: "pytorch-notebook", GPUClass: "l40s-48gb", GPUCount: gpus,
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return w
}

func (h *harness) state(p domain.Principal, id uuid.UUID) domain.State {
	h.t.Helper()
	w, err := h.svc.GetWorkload(context.Background(), p, id)
	if domain.IsNotFound(err) {
		return domain.StateDeleted
	}
	if err != nil {
		h.t.Fatal(err)
	}
	return w.State
}

func (h *harness) expect(p domain.Principal, id uuid.UUID, want domain.State) {
	h.t.Helper()
	if got := h.state(p, id); got != want {
		h.t.Fatalf("workload %s: state = %s, want %s", id, got, want)
	}
}

func TestLifecycleWithContention(t *testing.T) {
	h := newHarness(t, map[string]int{"l40s-48gb": 2})
	ctx := context.Background()

	acme := h.create(testutil.Acme, "big", 2)
	h.reconcile() // admitted, applied, observed ready in the same pass
	h.expect(testutil.Acme, acme.ID, domain.StateRunning)

	globex := h.create(testutil.Globex, "small", 1)
	h.reconcile()
	h.expect(testutil.Globex, globex.ID, domain.StateQueued)

	if _, err := h.svc.StopWorkload(ctx, testutil.Acme, acme.ID); err != nil {
		t.Fatal(err)
	}
	h.reconcile() // GPUs are still held while stopping; the park step finishes the stop
	h.expect(testutil.Acme, acme.ID, domain.StateStopped)
	h.expect(testutil.Globex, globex.ID, domain.StateQueued)

	h.reconcile() // capacity freed → globex admitted
	h.expect(testutil.Globex, globex.ID, domain.StateRunning)

	if _, err := h.svc.DeleteWorkload(ctx, testutil.Acme, acme.ID); err != nil {
		t.Fatal(err)
	}
	h.reconcile()
	h.expect(testutil.Acme, acme.ID, domain.StateDeleted)
}

func TestStuckProvisioningFailsAfterTimeoutAndReleasesGPUs(t *testing.T) {
	h := newHarness(t, map[string]int{"l40s-48gb": 1})
	w := h.create(testutil.Acme, "stuck", 1)
	h.backend.SetObserved(w.ID, cluster.ObservedProgressing)

	h.reconcile()
	h.expect(testutil.Acme, w.ID, domain.StateProvisioning)

	h.clock.Advance(11 * time.Minute)
	h.reconcile()
	h.expect(testutil.Acme, w.ID, domain.StateFailed)

	// The failed workload no longer holds capacity, so new work is admitted.
	next := h.create(testutil.Acme, "next", 1)
	h.reconcile()
	h.expect(testutil.Acme, next.ID, domain.StateRunning)
}

func TestWorkloadThatVanishesIsMarkedFailed(t *testing.T) {
	h := newHarness(t, map[string]int{"l40s-48gb": 1})
	w := h.create(testutil.Acme, "nb", 1)
	h.reconcile()
	h.expect(testutil.Acme, w.ID, domain.StateRunning)

	h.backend.SetObserved(w.ID, cluster.ObservedAbsent)
	h.reconcile()
	h.expect(testutil.Acme, w.ID, domain.StateFailed)
}

func TestDeletingQueuedWorkloadNeedsNoClusterObjects(t *testing.T) {
	h := newHarness(t, map[string]int{"l40s-48gb": 0})
	w := h.create(testutil.Acme, "never-ran", 1)
	h.reconcile()
	h.expect(testutil.Acme, w.ID, domain.StateQueued)
	if _, err := h.svc.DeleteWorkload(context.Background(), testutil.Acme, w.ID); err != nil {
		t.Fatal(err)
	}
	h.reconcile()
	h.expect(testutil.Acme, w.ID, domain.StateDeleted)
}
