package agent_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/agent"
	"github.com/tensora/tensora/controlplane/internal/agent/drivers/sim"
	"github.com/tensora/tensora/controlplane/internal/agent/protocol"
	"github.com/tensora/tensora/controlplane/internal/domain"
)

// conn records what the agent sends and answers with resp.
type conn struct {
	sent []protocol.SyncRequest
	resp protocol.SyncResponse
}

func (c *conn) Sync(_ context.Context, req protocol.SyncRequest) (protocol.SyncResponse, error) {
	c.sent = append(c.sent, req)
	return c.resp, nil
}

func macInventory(context.Context) (protocol.Inventory, error) {
	return protocol.Inventory{OS: "darwin", Arch: "arm64", CPUs: 11, MemoryMiB: 36864, UsableCPUs: 9, UsableMemoryMiB: 27648,
		Accelerators: []domain.Accelerator{{Kind: domain.AcceleratorApple, Model: "Apple M3 Pro", Count: 1, MemoryMiB: 36864}}}, nil
}

func newAgent(t *testing.T) (*agent.Agent, *conn, *sim.Driver) {
	t.Helper()
	c := &conn{}
	d := sim.New(domain.AcceleratorApple, domain.AcceleratorCPU)
	a := agent.New(c, []agent.Driver{d}, macInventory, "test", slog.New(slog.DiscardHandler))
	if _, err := a.Sync(context.Background(), 0, true); err != nil { // detects the inventory
		t.Fatal(err)
	}
	return a, c, d
}

func gpuWorkload(mode protocol.Mode) protocol.Desired {
	return protocol.Desired{ID: uuid.New(), Mode: mode, Kind: domain.KindNotebook, Driver: domain.DriverSim,
		Accelerator: domain.AcceleratorApple, Devices: 1, VCPUs: 6, MemoryMiB: 24576}
}

func statuses(t *testing.T, a *agent.Agent, c *conn) map[uuid.UUID]protocol.Status {
	t.Helper()
	if _, err := a.Sync(context.Background(), 1, false); err != nil {
		t.Fatal(err)
	}
	out := map[uuid.UUID]protocol.Status{}
	for _, st := range c.sent[len(c.sent)-1].Statuses {
		out[st.ID] = st
	}
	return out
}

// The agent double-checks what it's given: a driver it doesn't have, weak isolation for someone
// else's workload, and a GPU another workload holds are all refused, with a code.
func TestAgentRefusesWhatItShouldnt(t *testing.T) {
	a, c, _ := newAgent(t)
	first, second := gpuWorkload(protocol.ModeRun), gpuWorkload(protocol.ModeRun)
	noDriver := gpuWorkload(protocol.ModeRun)
	noDriver.Driver, noDriver.Devices = domain.DriverDocker, 0
	guest := gpuWorkload(protocol.ModeRun)
	guest.StrongIsolation, guest.Devices = true, 0
	a.Apply(context.Background(), []protocol.Desired{first, second, noDriver, guest})

	got := statuses(t, a, c)
	for id, want := range map[uuid.UUID]protocol.Status{
		first.ID:    {State: protocol.StateReady},
		second.ID:   {State: protocol.StateProgressing, Code: protocol.CodeDevicesBusy},
		noDriver.ID: {State: protocol.StateFailed, Code: protocol.CodeNoDriver},
		guest.ID:    {State: protocol.StateFailed, Code: protocol.CodeIsolation},
	} {
		if g := got[id]; g.State != want.State || g.Code != want.Code {
			t.Errorf("workload %s: %+v, want %+v", id, g, want)
		}
	}
}

// Parks and removals happen before runs, so a GPU freed in the same desired state is used at once.
// Anything the control plane doesn't list is stopped and kept: data is deleted only when a workload
// is listed for removal.
func TestAgentFreesBeforeItStarts(t *testing.T) {
	a, c, d := newAgent(t)
	ctx := context.Background()
	old, stray := gpuWorkload(protocol.ModeRun), gpuWorkload(protocol.ModeRun)
	stray.Devices = 0
	a.Apply(ctx, []protocol.Desired{old, stray})
	old.Mode = protocol.ModePark
	next := gpuWorkload(protocol.ModeRun)
	a.Apply(ctx, []protocol.Desired{next, old}) // stray is no longer listed

	got := statuses(t, a, c)
	if got[old.ID].State != protocol.StateStopped || got[next.ID].State != protocol.StateReady {
		t.Errorf("after parking one and running another: %+v", got)
	}
	if got[stray.ID].State != protocol.StateStopped {
		t.Errorf("a workload the control plane no longer lists: %+v; want it stopped and kept", got[stray.ID])
	}
	sts, _ := d.Statuses(ctx)
	if len(sts) != 3 {
		t.Errorf("the driver has %d workloads, want 3: nothing removed without being listed for removal", len(sts))
	}
	gone := stray
	gone.Mode = protocol.ModeRemove
	a.Apply(ctx, []protocol.Desired{next, old, gone})
	if _, ok := statuses(t, a, c)[stray.ID]; ok {
		t.Error("a workload listed for removal is still there")
	}
}

// failing wraps a driver and fails what it's told to.
type failing struct {
	agent.Driver
	stop, remove bool
}

func (f failing) Stop(ctx context.Context, id uuid.UUID) error {
	if f.stop {
		return context.DeadlineExceeded
	}
	return f.Driver.Stop(ctx, id)
}

func (f failing) Remove(ctx context.Context, id uuid.UUID) error {
	if f.remove {
		return context.DeadlineExceeded
	}
	return f.Driver.Remove(ctx, id)
}

// A GPU is free only once the workload holding it has actually stopped: while its removal or
// parking is stuck, nothing else starts on it.
func TestGPUStaysBusyUntilItsWorkloadStops(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []protocol.Mode{protocol.ModeRemove, protocol.ModePark} {
		c := &conn{}
		d := sim.New(domain.AcceleratorApple, domain.AcceleratorCPU)
		a := agent.New(c, []agent.Driver{failing{Driver: d, stop: true, remove: true}}, macInventory, "test", slog.New(slog.DiscardHandler))
		if _, err := a.Sync(ctx, 0, true); err != nil {
			t.Fatal(err)
		}
		owner := gpuWorkload(protocol.ModeRun)
		a.Apply(ctx, []protocol.Desired{owner})
		leaving := protocol.Desired{ID: owner.ID, Mode: mode, Kind: owner.Kind, Driver: owner.Driver} // no devices described
		guest := gpuWorkload(protocol.ModeRun)
		a.Apply(ctx, []protocol.Desired{leaving, guest})
		if got := statuses(t, a, c)[guest.ID]; got.Code != protocol.CodeDevicesBusy {
			t.Errorf("%s stuck: the next workload got %+v; want it waiting for the GPU", mode, got)
		}
	}
}

// A notebook or VM is never moved to another driver: that would lose its data. An endpoint, which
// keeps nothing, can move.
func TestAgentNeverSwitchesDriversForData(t *testing.T) {
	ctx := context.Background()
	c := &conn{}
	first, second := sim.New(domain.AcceleratorCPU), renamed{sim.New(domain.AcceleratorCPU), domain.DriverMacOSNative}
	a := agent.New(c, []agent.Driver{first, second}, macInventory, "test", slog.New(slog.DiscardHandler))
	if _, err := a.Sync(ctx, 0, true); err != nil {
		t.Fatal(err)
	}
	nb := protocol.Desired{ID: uuid.New(), Mode: protocol.ModeRun, Kind: domain.KindNotebook, Driver: domain.DriverSim, Accelerator: domain.AcceleratorCPU}
	ep := nb
	ep.ID, ep.Kind = uuid.New(), domain.KindEndpoint
	a.Apply(ctx, []protocol.Desired{nb, ep})
	nb.Driver, ep.Driver = domain.DriverMacOSNative, domain.DriverMacOSNative
	nb.Mode = protocol.ModeRun
	a.Apply(ctx, []protocol.Desired{nb, ep})
	got := statuses(t, a, c)
	if got[nb.ID].Code != protocol.CodeInvalid {
		t.Errorf("a notebook asked to switch drivers: %+v; want it refused", got[nb.ID])
	}
	if sts, _ := first.Statuses(ctx); len(sts) != 1 || sts[0].ID != nb.ID {
		t.Errorf("the first driver has %+v; want the notebook kept and the endpoint moved", sts)
	}
	if got[ep.ID].State != protocol.StateReady {
		t.Errorf("the endpoint after moving: %+v", got[ep.ID])
	}
}

// renamed gives a driver another name, so a test has two.
type renamed struct {
	agent.Driver
	name string
}

func (r renamed) Name() string { return r.name }

// The inventory goes with the first sync and when the control plane asks, not every time, and it
// names the agent's drivers.
func TestAgentSendsItsInventoryWhenNeeded(t *testing.T) {
	a, c, _ := newAgent(t)
	ctx := context.Background()
	if inv := c.sent[0].Inventory; inv == nil || len(inv.Drivers) != 1 || inv.Drivers[0].Name != domain.DriverSim || inv.AgentVersion != "test" {
		t.Fatalf("first sync's inventory = %+v", c.sent[0].Inventory)
	}
	if _, err := a.Sync(ctx, 1, false); err != nil || c.sent[1].Inventory != nil {
		t.Errorf("an unchanged inventory was sent again: %+v, %v", c.sent[1].Inventory, err)
	}
	if _, err := a.Sync(ctx, 1, true); err != nil || c.sent[2].Inventory == nil {
		t.Errorf("the inventory wasn't sent when asked: %v", err)
	}
	if c.sent[0].Version != protocol.Version {
		t.Errorf("version = %d", c.sent[0].Version)
	}
}

// slow blocks Run until released.
type slow struct {
	agent.Driver
	release chan struct{}
}

func (s slow) Run(ctx context.Context, d protocol.Desired) error {
	select {
	case <-s.release:
	case <-ctx.Done():
	}
	return s.Driver.Run(ctx, d)
}

// counting answers every sync with the same desired state, and counts them.
type counting struct {
	mu      sync.Mutex
	n       int
	desired []protocol.Desired
}

func (c *counting) Sync(ctx context.Context, _ protocol.SyncRequest) (protocol.SyncResponse, error) {
	c.mu.Lock()
	c.n++
	gen := uint64(c.n)
	c.mu.Unlock()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Millisecond): // a short long poll
	}
	return protocol.SyncResponse{Generation: gen, Workloads: c.desired}, nil
}

func (c *counting) syncs() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// A driver that takes a long time (pulling a large image) doesn't stop the agent from syncing, so a
// busy machine never looks offline.
func TestSlowDriversDontStopSyncing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	c := &counting{desired: []protocol.Desired{gpuWorkload(protocol.ModeRun)}}
	a := agent.New(c, []agent.Driver{slow{sim.New(domain.AcceleratorApple), release}}, macInventory, "test", slog.New(slog.DiscardHandler))
	done := make(chan struct{})
	go func() { _ = a.Run(ctx); close(done) }()
	time.Sleep(200 * time.Millisecond)
	if n := c.syncs(); n < 5 {
		t.Errorf("%d syncs while a driver call was running; want the agent to keep reporting", n)
	}
	close(release)
	cancel()
	<-done
}

// A workload held by two drivers (a move that didn't finish) holds every device until it's sorted
// out, and an endpoint's stray copy is removed.
func TestAWorkloadHeldTwiceIsSortedOut(t *testing.T) {
	ctx := context.Background()
	c := &conn{}
	first, second := sim.New(domain.AcceleratorApple), renamed{sim.New(domain.AcceleratorApple), domain.DriverMacOSNative}
	a := agent.New(c, []agent.Driver{first, second}, macInventory, "test", slog.New(slog.DiscardHandler))
	if _, err := a.Sync(ctx, 0, true); err != nil {
		t.Fatal(err)
	}
	ep := gpuWorkload(protocol.ModeRun)
	ep.Kind = domain.KindEndpoint
	_ = first.Run(ctx, ep)
	moved := ep
	moved.Driver = domain.DriverMacOSNative
	_ = second.Run(ctx, moved)

	other := gpuWorkload(protocol.ModeRun)
	a.Apply(ctx, []protocol.Desired{moved, other})
	if got := statuses(t, a, c)[other.ID]; got.Code != protocol.CodeDevicesBusy {
		t.Errorf("another workload while one is held twice: %+v; want it waiting for the GPU", got)
	}
	if sts, _ := first.Statuses(ctx); len(sts) != 0 {
		t.Errorf("the endpoint's stray copy is still in the first driver: %+v", sts)
	}
	if got := statuses(t, a, c)[moved.ID]; got.State != protocol.StateReady || got.Code != "" {
		t.Errorf("the endpoint, once held only by its driver: %+v; want it running, not waiting on itself", got)
	}
}
