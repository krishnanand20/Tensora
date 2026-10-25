package agents_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/agent/protocol"
	"github.com/tensora/tensora/controlplane/internal/cluster"
	"github.com/tensora/tensora/controlplane/internal/cluster/agents"
	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/store"
	"github.com/tensora/tensora/controlplane/internal/testutil"
)

type fixture struct {
	st      store.Store
	hub     *agents.Hub
	machine domain.Machine
	owner   domain.Tenant
	guest   domain.Tenant
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	st := testutil.SeededStore(t)
	ctx := context.Background()
	m, err := agents.RegisterLocal(ctx, st, uuid.Nil, "internal", "studio", "darwin", "arm64", domain.RegionLocal, testutil.T0)
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{st: st, machine: m}
	_ = st.InTx(ctx, func(q store.Queries) error {
		f.owner, _ = q.TenantBySlug(ctx, "internal")
		f.guest, _ = q.TenantBySlug(ctx, "acme")
		return nil
	})
	f.hub = agents.New(st, testutil.Catalog(t), agents.Options{PollTimeout: 50 * time.Millisecond, AllowSim: true},
		slog.New(slog.DiscardHandler), nil)
	return f
}

// mac is a 36 GB Apple Silicon Mac's report with the sim driver.
func mac() *protocol.Inventory {
	return &protocol.Inventory{OS: "darwin", Arch: "arm64", CPUs: 11, MemoryMiB: 36 * 1024,
		Accelerators: []domain.Accelerator{{Kind: domain.AcceleratorApple, Model: "Apple M3 Pro", Count: 1, MemoryMiB: 36 * 1024}},
		UsableCPUs:   9, UsableMemoryMiB: 27 * 1024,
		Drivers: []protocol.Driver{{Name: domain.DriverSim, Accelerators: []domain.AcceleratorKind{domain.AcceleratorApple, domain.AcceleratorCPU}}}}
}

// place stores a workload placed on the fixture's machine, in state.
func (f fixture) place(t *testing.T, tenant domain.Tenant, kind domain.WorkloadKind, class string, state domain.State) domain.Workload {
	t.Helper()
	return f.placeOn(t, f.machine, tenant, kind, class, state)
}

func (f fixture) placeOn(t *testing.T, m domain.Machine, tenant domain.Tenant, kind domain.WorkloadKind, class string, state domain.State) domain.Workload {
	t.Helper()
	ctx := context.Background()
	image := map[domain.WorkloadKind]string{domain.KindNotebook: "pytorch-notebook", domain.KindEndpoint: "openai-endpoint", domain.KindVM: "ubuntu-24.04-cuda"}[kind]
	w := domain.Workload{ID: uuid.New(), TenantID: tenant.ID, Name: "w", Kind: kind, State: domain.StatePending, ImageID: image,
		GPUClass: class, GPUCount: 1, Params: map[string]string{domain.ParamModelURI: "hf://Qwen/Qwen2.5-0.5B-Instruct"},
		CreatedBy: "test", CreatedAt: testutil.T0, UpdatedAt: testutil.T0, Region: domain.RegionLocal}
	err := f.st.InTx(ctx, func(q store.Queries) error {
		if err := q.CreateWorkload(ctx, w); err != nil {
			return err
		}
		driver := domain.DriverSim
		if tenant.ID != m.TenantID {
			driver = domain.DriverMacOSVZ
		}
		if err := q.SetPlacement(ctx, w.ID, class, m.ID, driver); err != nil {
			return err
		}
		w.PlacedClass, w.MachineID, w.Driver = class, m.ID, driver
		if state != domain.StatePending {
			w.State = state
			return q.UpdateWorkloadState(ctx, w.ID, domain.StatePending, state, testutil.T0)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestSyncSendsDesiredStateFromTheStore(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	running := f.place(t, f.owner, domain.KindNotebook, "apple-silicon-32gb", domain.StateProvisioning)
	stopped := f.place(t, f.owner, domain.KindNotebook, "cpu-2c-8g", domain.StateStopping)
	stoppedEndpoint := f.place(t, f.owner, domain.KindEndpoint, "apple-silicon-32gb", domain.StateStopping)
	deleting := f.place(t, f.owner, domain.KindNotebook, "cpu-2c-8g", domain.StateDeleting)

	resp, err := f.hub.Conn(f.machine.ID).Sync(ctx, protocol.SyncRequest{Version: protocol.Version, Inventory: mac()})
	if err != nil {
		t.Fatal(err)
	}
	modes := map[uuid.UUID]protocol.Desired{}
	for _, d := range resp.Workloads {
		modes[d.ID] = d
	}
	for id, want := range map[uuid.UUID]protocol.Mode{
		running.ID: protocol.ModeRun, stopped.ID: protocol.ModePark, stoppedEndpoint.ID: protocol.ModeRemove, deleting.ID: protocol.ModeRemove,
	} {
		if got := modes[id].Mode; got != want {
			t.Errorf("workload %s: mode %q, want %q", id, got, want)
		}
	}
	d := modes[running.ID]
	if d.Driver != domain.DriverSim || d.Accelerator != domain.AcceleratorApple || d.Devices != 1 || d.VCPUs != 6 ||
		d.MemoryMiB != 24*1024 || d.Runtime.Ref != "jupyterlab-pytorch-mps" || d.StrongIsolation {
		t.Errorf("desired = %+v", d)
	}
	if resp.SendInventory {
		t.Error("the inventory was sent; the hub mustn't ask again")
	}

	// The inventory is recorded on the machine, never its owner, name, or region.
	_ = f.st.InTx(ctx, func(q store.Queries) error {
		m, err := q.MachineByID(ctx, f.machine.ID)
		if err != nil || m.CPUs != 11 || m.MemoryMiB != 36*1024 || len(m.Accelerators) != 1 || m.LastSeenAt.IsZero() ||
			m.TenantID != f.machine.TenantID || m.Region != domain.RegionLocal {
			t.Errorf("machine after the first sync = %+v, %v", m, err)
		}
		return nil
	})
}

// A workload that isn't the machine owner's is marked as needing strong isolation, so the agent
// checks the driver too.
func TestGuestsNeedStrongIsolation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pool := domain.Machine{ID: uuid.New(), TenantID: f.owner.ID, Name: "pool-mac", Region: domain.RegionLocal, OS: "darwin",
		Arch: "arm64", Sharing: domain.SharingCommunity, CreatedAt: testutil.T0}
	if err := f.st.InTx(ctx, func(q store.Queries) error { return q.CreateMachine(ctx, pool) }); err != nil {
		t.Fatal(err)
	}
	guests := f.placeOn(t, pool, f.guest, domain.KindVM, "apple-silicon-32gb", domain.StateProvisioning)
	owners := f.placeOn(t, pool, f.owner, domain.KindNotebook, "cpu-2c-8g", domain.StateProvisioning)
	resp, err := f.hub.Conn(pool.ID).Sync(ctx, protocol.SyncRequest{Version: protocol.Version, Inventory: mac()})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range resp.Workloads {
		if want := d.ID == guests.ID; d.StrongIsolation != want || (d.ID != guests.ID && d.ID != owners.ID) {
			t.Errorf("workload %s: strong isolation %v, want %v", d.ID, d.StrongIsolation, want)
		}
	}
}

func TestInventoryIsCheckedAndDriversFiltered(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	bad := mac()
	bad.UsableMemoryMiB = bad.MemoryMiB + 1
	if _, err := f.hub.Conn(f.machine.ID).Sync(ctx, protocol.SyncRequest{Version: protocol.Version, Inventory: bad}); err == nil {
		t.Error("more usable memory than the machine has was accepted")
	}
	if _, err := f.hub.Conn(f.machine.ID).Sync(ctx, protocol.SyncRequest{Version: 99}); err == nil {
		t.Error("another protocol version was accepted")
	}

	noSim := agents.New(f.st, testutil.Catalog(t), agents.Options{PollTimeout: 10 * time.Millisecond}, slog.New(slog.DiscardHandler), nil)
	inv := mac()
	inv.Drivers = append(inv.Drivers, protocol.Driver{Name: "made-up", Accelerators: []domain.AcceleratorKind{domain.AcceleratorApple}},
		protocol.Driver{Name: domain.DriverMacOSNative, Accelerators: []domain.AcceleratorKind{domain.AcceleratorApple, domain.AcceleratorNVIDIA}})
	if _, err := noSim.Conn(f.machine.ID).Sync(ctx, protocol.SyncRequest{Version: protocol.Version, Inventory: inv}); err != nil {
		t.Fatal(err)
	}
	c, _ := noSim.Capacity(ctx)
	if len(c.Machines) != 1 || len(c.Machines[0].Drivers) != 1 || c.Machines[0].Drivers[0].Name != domain.DriverMacOSNative ||
		len(c.Machines[0].Drivers[0].Accelerators) != 1 {
		t.Fatalf("capacity = %+v; want only macos-native, with only the accelerators it can use", c.Machines)
	}
	if c.Classes != nil || c.Machines[0].Devices["apple-silicon-32gb"] != 1 || c.Machines[0].VCPUs != 9 {
		t.Errorf("capacity = %+v; want the Mac's GPU as apple-silicon-32gb and its usable CPUs", c)
	}
}

func TestMachinesInOtherRegionsAreRefused(t *testing.T) {
	f := newFixture(t)
	other := agents.New(f.st, testutil.Catalog(t), agents.Options{Region: "eu-fra-1"}, slog.New(slog.DiscardHandler), nil)
	_, err := other.Conn(f.machine.ID).Sync(context.Background(), protocol.SyncRequest{Version: protocol.Version, Inventory: mac()})
	if code, _ := domain.CodeOf(err); code != domain.CodeForbidden {
		t.Errorf("a machine syncing with another region's control plane: %v", err)
	}
}

// An up-to-date agent waits for a change, and a nudge answers it at once.
func TestSyncWaitsForAChange(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	hub := agents.New(f.st, testutil.Catalog(t), agents.Options{PollTimeout: 5 * time.Second, AllowSim: true}, slog.New(slog.DiscardHandler), nil)
	conn := hub.Conn(f.machine.ID)
	first, err := conn.Sync(ctx, protocol.SyncRequest{Version: protocol.Version, Inventory: mac()})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan protocol.SyncResponse)
	go func() {
		resp, _ := conn.Sync(ctx, protocol.SyncRequest{Version: protocol.Version, Generation: first.Generation})
		done <- resp
	}()
	select {
	case <-done:
		t.Fatal("an up-to-date agent was answered without a change")
	case <-time.After(100 * time.Millisecond):
	}
	w := f.place(t, f.owner, domain.KindNotebook, "cpu-2c-8g", domain.StateProvisioning)
	_ = hub.Run(ctx, cluster.Spec{Workload: w})
	select {
	case resp := <-done:
		if resp.Generation == first.Generation || len(resp.Workloads) != 1 {
			t.Errorf("after a nudge: %+v", resp)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a nudge didn't wake the waiting agent")
	}
}

func TestObserveSaysWhatTheAgentReported(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ready, failed, odd, missing := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ref := func(id uuid.UUID) cluster.Ref { return cluster.Ref{ID: id, MachineID: f.machine.ID} }

	if obs, reason, _ := f.hub.Observe(ctx, ref(ready)); obs != cluster.ObservedProgressing || reason != "" {
		t.Errorf("before the machine syncs, just after a restart: %s %q; want progressing, with nothing to say yet", obs, reason)
	}
	_, err := f.hub.Conn(f.machine.ID).Sync(ctx, protocol.SyncRequest{Version: protocol.Version, Inventory: mac(), Statuses: []protocol.Status{
		{ID: ready, State: protocol.StateReady},
		{ID: failed, State: protocol.StateFailed, Code: protocol.CodeOutOfMemory},
		{ID: odd, State: "exploded", Code: "rm -rf / at t-acme"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[uuid.UUID]struct {
		obs    cluster.Observed
		reason string
	}{
		ready:   {cluster.ObservedReady, ""},
		failed:  {cluster.ObservedFailed, "it ran out of memory"},
		odd:     {cluster.ObservedFailed, "starting on its machine"},
		missing: {cluster.ObservedAbsent, ""},
	} {
		obs, reason, err := f.hub.Observe(ctx, ref(id))
		if err != nil || obs != want.obs || reason != want.reason {
			t.Errorf("Observe = %s %q %v; want %s %q", obs, reason, err, want.obs, want.reason)
		}
	}
	if obs, _, _ := f.hub.Observe(ctx, cluster.Ref{ID: ready}); obs != cluster.ObservedAbsent {
		t.Errorf("a workload never placed on a machine: %s; want absent", obs)
	}
	if got := protocol.Reason("rm -rf / at t-acme"); got != "its machine reported a problem" {
		t.Errorf("an unknown code's reason = %q; an agent's words never reach a timeline", got)
	}
}

// A machine's agent that stops syncing goes offline: it isn't offered for placement, and its
// workloads read as unknown, not absent.
func TestOfflineMachines(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	clock := testutil.NewClock(testutil.T0)
	hub := agents.New(f.st, testutil.Catalog(t), agents.Options{PollTimeout: time.Millisecond, OfflineAfter: time.Minute, AllowSim: true},
		slog.New(slog.DiscardHandler), clock.Now)
	if _, err := hub.Conn(f.machine.ID).Sync(ctx, protocol.SyncRequest{Version: protocol.Version, Inventory: mac()}); err != nil {
		t.Fatal(err)
	}
	if c, _ := hub.Capacity(ctx); len(c.Machines) != 1 {
		t.Fatalf("online: %+v", c)
	}
	clock.Advance(2 * time.Minute)
	if c, _ := hub.Capacity(ctx); len(c.Machines) != 0 {
		t.Errorf("offline machine offered: %+v", c)
	}
	if obs, reason, _ := hub.Observe(ctx, cluster.Ref{ID: uuid.New(), MachineID: f.machine.ID}); obs != cluster.ObservedUnknown || reason != agents.Offline {
		t.Errorf("offline: %s %q", obs, reason)
	}
	// Gone too long: its workloads aren't running, so they mustn't stay running and accounted.
	clock.Advance(10 * time.Minute)
	if obs, reason, _ := hub.Observe(ctx, cluster.Ref{ID: uuid.New(), MachineID: f.machine.ID}); obs != cluster.ObservedFailed ||
		reason != "its machine has been offline for over 10m0s" {
		t.Errorf("lost: %s %q", obs, reason)
	}
}

// Right after a restart, while machines reconnect, the backend says it's settling, so nothing is
// queued for a machine that's about to come back.
func TestCapacitySettlesAfterARestart(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	clock := testutil.NewClock(testutil.T0)
	hub := agents.New(f.st, testutil.Catalog(t), agents.Options{PollTimeout: time.Millisecond, AllowSim: true}, slog.New(slog.DiscardHandler), clock.Now)
	if c, _ := hub.Capacity(ctx); !c.Settling {
		t.Error("just started, with the machine not yet back: want settling")
	}
	if _, err := hub.Conn(f.machine.ID).Sync(ctx, protocol.SyncRequest{Version: protocol.Version, Inventory: mac()}); err != nil {
		t.Fatal(err)
	}
	if c, _ := hub.Capacity(ctx); c.Settling || len(c.Machines) != 1 {
		t.Errorf("every machine is back: %+v", c)
	}
	quiet := agents.New(f.st, testutil.Catalog(t), agents.Options{}, slog.New(slog.DiscardHandler), clock.Now)
	clock.Advance(time.Minute)
	if c, _ := quiet.Capacity(ctx); c.Settling {
		t.Error("still settling after the reconnect window")
	}
}

// An agent's report must fit its machine: GPU counts decide who shares nothing, so they're never
// taken on trust.
func TestInventoryMustFitTheMachine(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for name, change := range map[string]func(*protocol.Inventory){
		"two Apple GPUs":           func(inv *protocol.Inventory) { inv.Accelerators[0].Count = 2 },
		"an Apple GPU on Linux":    func(inv *protocol.Inventory) { inv.OS = "linux" },
		"more GPU memory than RAM": func(inv *protocol.Inventory) { inv.Accelerators[0].MemoryMiB = inv.MemoryMiB + 1 },
		"an NVIDIA GPU on a Mac": func(inv *protocol.Inventory) {
			inv.Accelerators = append(inv.Accelerators, domain.Accelerator{Kind: domain.AcceleratorNVIDIA, Model: "NVIDIA L40S", Count: 1})
		},
	} {
		inv := mac()
		change(inv)
		if _, err := f.hub.Conn(f.machine.ID).Sync(ctx, protocol.SyncRequest{Version: protocol.Version, Inventory: inv}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A driver for another OS is dropped.
	inv := mac()
	inv.Drivers = append(inv.Drivers, protocol.Driver{Name: domain.DriverDocker, Accelerators: []domain.AcceleratorKind{domain.AcceleratorCPU}})
	if _, err := f.hub.Conn(f.machine.ID).Sync(ctx, protocol.SyncRequest{Version: protocol.Version, Inventory: inv}); err != nil {
		t.Fatal(err)
	}
	c, _ := f.hub.Capacity(ctx)
	for _, d := range c.Machines[0].Drivers {
		if d.Name == domain.DriverDocker {
			t.Error("docker was accepted on a Mac")
		}
	}
}

func TestRegisterLocal(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	again, err := agents.RegisterLocal(ctx, f.st, uuid.Nil, "internal", "studio", "darwin", "arm64", domain.RegionLocal, testutil.T0)
	if err != nil || again.ID != f.machine.ID {
		t.Errorf("registering again = %+v, %v; want the same machine", again, err)
	}
	if _, err := agents.RegisterLocal(ctx, f.st, uuid.Nil, "internal", "studio", "darwin", "arm64", "eu-fra-1", testutil.T0); err == nil {
		t.Error("a machine moved region")
	}
	if _, err := agents.RegisterLocal(ctx, f.st, uuid.Nil, "nobody", "studio", "darwin", "arm64", domain.RegionLocal, testutil.T0); !domain.IsNotFound(err) {
		t.Errorf("an owner tenant that doesn't exist: %v; want not found", err)
	}
	if f.machine.Sharing != domain.SharingPrivate {
		t.Errorf("a registered machine is %s; machines are private by default", f.machine.Sharing)
	}

	// Once its ID is known, the machine is found by ID: a new hostname is only a new label, never
	// a new, empty machine whose agent would think its workloads were gone.
	renamed, err := agents.RegisterLocal(ctx, f.st, f.machine.ID, "internal", "studio-on-wifi", "darwin", "arm64", domain.RegionLocal, testutil.T0)
	if err != nil || renamed.ID != f.machine.ID {
		t.Errorf("after a hostname change = %+v, %v; want the same machine", renamed, err)
	}
	if _, err := agents.RegisterLocal(ctx, f.st, f.machine.ID, "acme", "studio", "darwin", "arm64", domain.RegionLocal, testutil.T0); err == nil {
		t.Error("the machine's owner changed")
	}
	if _, err := agents.RegisterLocal(ctx, f.st, f.machine.ID, "internal", "studio", "darwin", "arm64", "eu-fra-1", testutil.T0); err == nil {
		t.Error("the machine moved region")
	}
	fresh := uuid.New() // an ID from a database that was replaced
	if m, err := agents.RegisterLocal(ctx, f.st, fresh, "internal", "laptop", "darwin", "arm64", domain.RegionLocal, testutil.T0); err != nil || m.ID != fresh {
		t.Errorf("a saved ID this database doesn't have = %+v, %v; want it registered with that ID", m, err)
	}
}
