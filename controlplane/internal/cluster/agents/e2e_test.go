package agents_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/tensora/tensora/controlplane/internal/agent"
	"github.com/tensora/tensora/controlplane/internal/agent/drivers/sim"
	"github.com/tensora/tensora/controlplane/internal/agent/protocol"
	"github.com/tensora/tensora/controlplane/internal/cluster/agents"
	"github.com/tensora/tensora/controlplane/internal/controller"
	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/service"
	"github.com/tensora/tensora/controlplane/internal/store"
	"github.com/tensora/tensora/controlplane/internal/testutil"
)

// The whole path in one process, as in Solo: a notebook is placed on this machine's GPU, the
// in-process agent runs it, and stop, start, and delete reach the agent too.
func TestInProcessAgentEndToEnd(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cat := testutil.Catalog(t)
	if err := f.st.InTx(ctx, func(q store.Queries) error {
		return q.SetQuota(ctx, domain.Quota{TenantID: f.owner.ID, GPUClass: "apple-silicon-32gb", MaxGPUs: 1})
	}); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.DiscardHandler)
	hub := agents.New(f.st, cat, agents.Options{PollTimeout: 20 * time.Millisecond, AllowSim: true}, log, nil)
	ctrl := controller.New(f.st, hub, cat, controller.Options{ProvisionTimeout: time.Minute}, log, nil)
	svc := service.New(f.st, cat, nil)
	inventory := func(context.Context) (protocol.Inventory, error) { return *mac(), nil }
	go func() {
		_ = agent.New(hub.Conn(f.machine.ID), []agent.Driver{sim.New(domain.AcceleratorApple, domain.AcceleratorCPU)},
			inventory, "test", log).Run(ctx)
	}()

	until := func(w domain.Workload, want domain.State) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if err := ctrl.ReconcileOnce(ctx); err != nil {
				t.Fatal(err)
			}
			got, err := svc.GetWorkloadHistory(ctx, testutil.Internal, w.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.State == want {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("workload never reached %s", want)
	}

	w, err := svc.CreateWorkload(ctx, testutil.Internal, service.CreateWorkloadInput{
		Name: "nb", Kind: domain.KindNotebook, ImageID: "pytorch-notebook", GPUClass: "apple-silicon-32gb", GPUCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	until(w, domain.StateRunning)

	placed, _ := svc.GetWorkload(ctx, testutil.Internal, w.ID)
	if placed.MachineID != f.machine.ID || placed.Driver != domain.DriverSim || placed.PlacedClass != "apple-silicon-32gb" {
		t.Errorf("placement = machine %s driver %q class %q", placed.MachineID, placed.Driver, placed.PlacedClass)
	}
	events, _ := svc.WorkloadEvents(ctx, testutil.Internal, w.ID)
	admitted := false
	for _, e := range events {
		admitted = admitted || e.Reason == "admitted: 1 × apple-silicon-32gb on studio with sim"
	}
	if !admitted {
		t.Errorf("the admission reason must say the machine and driver: %+v", events)
	}
	// The controller's first passes ran before the agent's first sync: it waited, rather than
	// queueing the workload for a machine that was about to connect.
	for _, e := range events {
		if e.To == domain.StateQueued {
			t.Errorf("queued while this machine was connecting: %q", e.Reason)
		}
	}

	if _, err := svc.StopWorkload(ctx, testutil.Internal, w.ID); err != nil {
		t.Fatal(err)
	}
	until(w, domain.StateStopped)
	if _, err := svc.StartWorkload(ctx, testutil.Internal, w.ID); err != nil {
		t.Fatal(err)
	}
	until(w, domain.StateRunning)
	if _, err := svc.DeleteWorkload(ctx, testutil.Internal, w.ID); err != nil {
		t.Fatal(err)
	}
	until(w, domain.StateDeleted)
}
