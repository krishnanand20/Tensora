// Package sim is a driver that runs nothing: workloads are ready as soon as they're applied. It's
// for development (`tensora dev` with TENSORA_CLUSTER_BACKEND=agent) and tests, and the control
// plane accepts it only when it's allowed to (TENSORA_ENV=dev). Its isolation counts as weak, so it
// never stands in for a strong driver.
package sim

import (
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/agent/protocol"
	"github.com/tensora/tensora/controlplane/internal/domain"
)

type Driver struct {
	accelerators []domain.AcceleratorKind

	mu        sync.Mutex
	workloads map[uuid.UUID]protocol.State
}

// New simulates the given accelerators, usually what the machine has, plus CPU.
func New(accelerators ...domain.AcceleratorKind) *Driver {
	return &Driver{accelerators: slices.Clone(accelerators), workloads: map[uuid.UUID]protocol.State{}}
}

func (d *Driver) Name() string { return domain.DriverSim }

func (d *Driver) Accelerators() []domain.AcceleratorKind { return slices.Clone(d.accelerators) }

func (d *Driver) Run(_ context.Context, w protocol.Desired) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.workloads[w.ID] = protocol.StateReady
	return nil
}

func (d *Driver) Stop(_ context.Context, id uuid.UUID) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.workloads[id]; ok {
		d.workloads[id] = protocol.StateStopped
	}
	return nil
}

func (d *Driver) Remove(_ context.Context, id uuid.UUID) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.workloads, id)
	return nil
}

func (d *Driver) Statuses(context.Context) ([]protocol.Status, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]protocol.Status, 0, len(d.workloads))
	for id, st := range d.workloads {
		out = append(out, protocol.Status{ID: id, State: st})
	}
	return out, nil
}
