// Package agent is the node agent (ADR-0013, ADR-0016). It reports its machine and its workloads to
// the control plane, and runs what it's told through drivers. Policy is the control plane's, but
// the agent checks what it's given: it refuses a driver it doesn't have, weak isolation for a
// workload that needs strong, and starting a workload while another holds the GPU it needs, so a
// bug or a race in the control plane can't put two users on one GPU. It deletes a workload's data
// only when told to remove that workload, never because a list left it out.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/agent/protocol"
	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/redact"
)

// Driver runs workloads one way (ADR-0013): natively on macOS, in containers, or in VMs. Run, Stop,
// and Remove must return promptly: long work, such as pulling an image, carries on in the
// background and shows as progressing in Statuses. The agent gives each call driverTimeout.
type Driver interface {
	// Name is the driver's name in domain.LookupDriver.
	Name() string
	// Accelerators are the kinds this driver can use on this machine.
	Accelerators() []domain.AcceleratorKind
	// Run starts the workload d describes, creating it if needed. Idempotent.
	Run(ctx context.Context, d protocol.Desired) error
	// Stop stops the workload and frees its GPUs, keeping everything it stored. Idempotent.
	Stop(ctx context.Context, id uuid.UUID) error
	// Remove removes the workload and everything it stored. Idempotent.
	Remove(ctx context.Context, id uuid.UUID) error
	// Statuses reports every workload the driver has. It's called while other calls are running.
	Statuses(ctx context.Context) ([]protocol.Status, error)
}

// Inventory detects the machine's hardware. Drivers and the agent's version are filled in by the
// agent.
type Inventory func(ctx context.Context) (protocol.Inventory, error)

type Agent struct {
	conn      protocol.Conn
	drivers   []Driver
	inventory Inventory
	version   string
	log       *slog.Logger

	mu      sync.Mutex
	inv     protocol.Inventory // the last detected
	invAt   time.Time
	refused map[uuid.UUID]protocol.Status // given but not run, reported until the next desired state
}

const (
	inventoryEvery = 5 * time.Minute
	firstRetry     = time.Second
	maxRetry       = 30 * time.Second
	// driverTimeout bounds every driver call, so one stuck call can't stall the machine.
	driverTimeout = 2 * time.Minute
)

func New(conn protocol.Conn, drivers []Driver, inventory Inventory, version string, log *slog.Logger) *Agent {
	return &Agent{conn: conn, drivers: drivers, inventory: inventory, version: version, log: log, refused: map[uuid.UUID]protocol.Status{}}
}

// Run syncs until ctx is cancelled, retrying with backoff when the control plane can't be reached.
// Applying runs separately, on the latest desired state, so the agent keeps reporting while a
// driver works: a busy machine never looks offline.
func (a *Agent) Run(ctx context.Context) error {
	a.log.Info("agent started", "drivers", len(a.drivers))
	latest := make(chan []protocol.Desired, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case desired := <-latest:
				a.Apply(ctx, desired)
			}
		}
	}()
	var gen uint64
	sendInventory := true
	wait := firstRetry
	for {
		resp, err := a.Sync(ctx, gen, sendInventory)
		select {
		case <-ctx.Done():
			<-done
			return nil // stopping
		default:
		}
		if err != nil {
			a.log.Warn("sync failed; retrying", "err", redact.Secrets(err.Error()), "in", wait)
			select {
			case <-ctx.Done():
			case <-time.After(wait):
			}
			wait, sendInventory = min(2*wait, maxRetry), true
			continue
		}
		wait = firstRetry
		gen, sendInventory = resp.Generation, resp.SendInventory
		select { // only the latest desired state matters
		case <-latest:
		default:
		}
		latest <- resp.Workloads
	}
}

// Sync sends one report: every workload's status, and the inventory when it changed or is asked for.
func (a *Agent) Sync(ctx context.Context, gen uint64, sendInventory bool) (protocol.SyncResponse, error) {
	req := protocol.SyncRequest{Version: protocol.Version, Generation: gen}
	inv, changed, err := a.detect(ctx)
	if err != nil {
		return protocol.SyncResponse{}, fmt.Errorf("inventory: %w", err)
	}
	if changed || sendInventory {
		req.Inventory = &inv
	}
	statuses, err := a.statuses(ctx)
	if err != nil {
		return protocol.SyncResponse{}, err
	}
	a.mu.Lock()
	for id, st := range statuses {
		// A refusal is what the control plane needs to hear about a workload it asked for, even
		// one a driver still has (such as a notebook it asked to move to another driver).
		if refused, ok := a.refused[id]; ok {
			st.Status = refused
		}
		req.Statuses = append(req.Statuses, st.Status)
	}
	for id, st := range a.refused {
		if _, ok := statuses[id]; !ok {
			req.Statuses = append(req.Statuses, st)
		}
	}
	a.mu.Unlock()
	return a.conn.Sync(ctx, req)
}

// detect refreshes the inventory every few minutes and reports whether it changed.
func (a *Agent) detect(ctx context.Context) (protocol.Inventory, bool, error) {
	a.mu.Lock()
	cached, at := a.inv, a.invAt
	a.mu.Unlock()
	if !at.IsZero() && time.Since(at) < inventoryEvery {
		return cached, false, nil
	}
	inv, err := a.inventory(ctx)
	if err != nil {
		return protocol.Inventory{}, false, err
	}
	inv.AgentVersion = a.version
	for _, d := range a.drivers {
		inv.Drivers = append(inv.Drivers, protocol.Driver{Name: d.Name(), Accelerators: d.Accelerators()})
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	changed := !reflect.DeepEqual(inv, a.inv)
	a.inv, a.invAt = inv, time.Now()
	return inv, changed, nil
}

// held is one workload a driver has. others are further drivers holding the same ID, which
// shouldn't happen (a move between drivers that didn't finish); such a workload is treated as
// holding every device until it's sorted out.
type held struct {
	protocol.Status
	driver Driver
	others []Driver
}

// statuses is every workload the drivers have, as they have it. The agent decides on this; refusals
// are added only to what it reports.
func (a *Agent) statuses(ctx context.Context) (map[uuid.UUID]held, error) {
	out := map[uuid.UUID]held{}
	for _, d := range a.drivers {
		sts, err := d.Statuses(ctx)
		if err != nil {
			return nil, fmt.Errorf("driver %s: %w", d.Name(), err)
		}
		for _, st := range sts {
			if h, ok := out[st.ID]; ok {
				h.others = append(h.others, d)
				if active(st.State) {
					h.State = st.State
				}
				out[st.ID] = h
				continue
			}
			out[st.ID] = held{Status: st, driver: d}
		}
	}
	return out, nil
}

func (a *Agent) driver(name string) Driver {
	for _, d := range a.drivers {
		if d.Name() == name {
			return d
		}
	}
	return nil
}

// Apply converges the machine to the desired state: removals and parks first, which free GPUs,
// then runs. A workload the drivers have that isn't listed is stopped and kept, never removed: the
// list can leave one out only by mistake (another machine's identity, a different database), and
// deleting data is never the safe guess. The operator's log says so.
func (a *Agent) Apply(ctx context.Context, desired []protocol.Desired) {
	refused := map[uuid.UUID]protocol.Status{}
	defer func() {
		a.mu.Lock()
		a.refused = refused
		a.mu.Unlock()
	}()
	refuse := func(id uuid.UUID, state protocol.State, code protocol.Code) {
		refused[id] = protocol.Status{ID: id, State: state, Code: code}
	}
	have, err := a.statuses(ctx)
	if err != nil {
		a.log.Error("can't read workloads; nothing applied", "err", redact.Secrets(err.Error()))
		return
	}
	order := map[protocol.Mode]int{protocol.ModeRemove: 0, protocol.ModePark: 1, protocol.ModeRun: 2}
	desired = slices.Clone(desired)
	slices.SortStableFunc(desired, func(x, y protocol.Desired) int { return order[x.Mode] - order[y.Mode] })

	listed := map[uuid.UUID]protocol.Desired{}
	for _, d := range desired {
		listed[d.ID] = d
	}
	for id, h := range have {
		if _, ok := listed[id]; !ok && active(h.State) {
			a.log.Warn("stopping a workload the control plane doesn't list; its data is kept until it's removed by hand",
				"workload", id, "driver", h.driver.Name())
			a.stop(ctx, h, refuse)
		}
	}
	for _, d := range desired {
		h, has := have[d.ID]
		switch d.Mode {
		case protocol.ModeRemove:
			if has {
				a.remove(ctx, h, refuse)
			}
			continue
		case protocol.ModePark:
			if has && active(h.State) {
				a.stop(ctx, h, refuse)
			}
			continue
		case protocol.ModeRun:
		default:
			refuse(d.ID, protocol.StateFailed, protocol.CodeInvalid)
			continue
		}
		drv := a.driver(d.Driver)
		spec, known := domain.LookupDriver(d.Driver)
		switch {
		case drv == nil || !known:
			refuse(d.ID, protocol.StateFailed, protocol.CodeNoDriver)
			continue
		case d.StrongIsolation && spec.Isolation != domain.IsolationStrong:
			refuse(d.ID, protocol.StateFailed, protocol.CodeIsolation)
			continue
		case has && (h.driver != drv || len(h.others) > 0):
			// Held by another driver than the one it's placed with. An endpoint keeps nothing, so
			// it moves once every other copy is gone; anything else would lose its data, so it's
			// refused.
			if d.Kind != domain.KindEndpoint {
				refuse(d.ID, protocol.StateFailed, protocol.CodeInvalid)
				continue
			}
			if !a.removeOthers(ctx, h, drv, refuse) {
				continue
			}
			current, err := a.statuses(ctx)
			if err != nil {
				refuse(d.ID, protocol.StateProgressing, protocol.CodeDriverProblem)
				continue
			}
			h, has = current[d.ID] // now held by drv alone, if at all
		}
		if has && active(h.State) {
			continue // already running; it was checked when it started
		}
		if !slices.Contains(drv.Accelerators(), d.Accelerator) || !slices.Contains(spec.Kinds, d.Kind) {
			refuse(d.ID, protocol.StateFailed, protocol.CodeInvalid)
			continue
		}
		if d.Devices > 0 {
			current, err := a.statuses(ctx)
			if err != nil {
				refuse(d.ID, protocol.StateProgressing, protocol.CodeDriverProblem)
				continue
			}
			if free := a.freeDevices(d.Accelerator, current, listed); free < d.Devices {
				refuse(d.ID, protocol.StateProgressing, protocol.CodeDevicesBusy)
				continue
			}
		}
		callCtx, cancel := context.WithTimeout(ctx, driverTimeout)
		err := drv.Run(callCtx, d)
		cancel()
		if err != nil {
			a.log.Error("driver couldn't run a workload", "workload", d.ID, "driver", d.Driver, "err", redact.Secrets(err.Error()))
			code := protocol.CodeDriverProblem
			var c codedError
			if errors.As(err, &c) {
				code = c.code
			}
			refuse(d.ID, protocol.StateFailed, code)
		}
	}
}

func (a *Agent) stop(ctx context.Context, h held, refuse func(uuid.UUID, protocol.State, protocol.Code)) {
	for _, d := range append([]Driver{h.driver}, h.others...) {
		callCtx, cancel := context.WithTimeout(ctx, driverTimeout)
		err := d.Stop(callCtx, h.ID)
		cancel()
		if err != nil {
			a.log.Error("driver couldn't stop a workload", "workload", h.ID, "driver", d.Name(), "err", redact.Secrets(err.Error()))
			refuse(h.ID, protocol.StateProgressing, protocol.CodeDriverProblem)
		}
	}
}

// remove removes the workload from every driver holding it, and reports whether that worked.
func (a *Agent) remove(ctx context.Context, h held, refuse func(uuid.UUID, protocol.State, protocol.Code)) bool {
	ok := true
	for _, d := range append([]Driver{h.driver}, h.others...) {
		ok = a.removeFrom(ctx, d, h.ID, refuse) && ok
	}
	return ok
}

// removeOthers removes an endpoint from every driver but keep, and reports whether that worked.
func (a *Agent) removeOthers(ctx context.Context, h held, keep Driver, refuse func(uuid.UUID, protocol.State, protocol.Code)) bool {
	ok := true
	for _, d := range append([]Driver{h.driver}, h.others...) {
		if d != keep {
			ok = a.removeFrom(ctx, d, h.ID, refuse) && ok
		}
	}
	return ok
}

func (a *Agent) removeFrom(ctx context.Context, d Driver, id uuid.UUID, refuse func(uuid.UUID, protocol.State, protocol.Code)) bool {
	callCtx, cancel := context.WithTimeout(ctx, driverTimeout)
	err := d.Remove(callCtx, id)
	cancel()
	if err != nil {
		a.log.Error("driver couldn't remove a workload", "workload", id, "driver", d.Name(), "err", redact.Secrets(err.Error()))
		refuse(id, protocol.StateProgressing, protocol.CodeRemovingFailed)
		return false
	}
	return true
}

func active(s protocol.State) bool { return s == protocol.StateProgressing || s == protocol.StateReady }

// freeDevices is how many devices of kind no active workload holds. Only a workload listed to run,
// with its devices described and held by one driver, counts as holding just its own; any other
// active workload (not listed, being parked or removed, described without devices, or held twice)
// is assumed to hold them all until it stops: two users never share a GPU.
func (a *Agent) freeDevices(kind domain.AcceleratorKind, current map[uuid.UUID]held, listed map[uuid.UUID]protocol.Desired) int {
	a.mu.Lock()
	total := 0
	for _, acc := range a.inv.Accelerators {
		if acc.Kind == kind {
			total += acc.Count
		}
	}
	a.mu.Unlock()
	used := 0
	for id, h := range current {
		if !active(h.State) {
			continue
		}
		d, ok := listed[id]
		switch {
		case !ok || d.Mode != protocol.ModeRun || d.Accelerator == "" || len(h.others) > 0:
			return 0
		case d.Accelerator == kind:
			used += d.Devices
		}
	}
	return total - used
}

// codedError is a driver error with a status code for the control plane.
type codedError struct {
	code protocol.Code
	err  error
}

func (e codedError) Error() string { return e.err.Error() }
func (e codedError) Unwrap() error { return e.err }

// WithCode marks a driver's error with the code the control plane should see, such as
// CodeImage when an image couldn't be fetched.
func WithCode(code protocol.Code, err error) error { return codedError{code: code, err: err} }
