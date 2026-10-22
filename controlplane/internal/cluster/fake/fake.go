// Package fake is an in-memory cluster.Backend for `tensora dev` and tests. Workloads become
// ready as soon as they are applied. It enforces the same JSON-type rules as the real API.
package fake

import (
	"context"
	"fmt"
	"maps"
	"sync"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/cluster"
	"github.com/tensora/tensora/controlplane/internal/domain"
)

type Backend struct {
	mu        sync.Mutex
	capacity  map[string]int
	running   map[uuid.UUID]bool // workload → desired running
	overrides map[uuid.UUID]cluster.Observed
}

func New(capacity map[string]int) *Backend {
	return &Backend{
		capacity:  maps.Clone(capacity),
		running:   map[uuid.UUID]bool{},
		overrides: map[uuid.UUID]cluster.Observed{},
	}
}

// SetObserved forces what Observe reports for a workload (e.g. to simulate a stuck or failed one).
func (b *Backend) SetObserved(id uuid.UUID, o cluster.Observed) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.overrides[id] = o
}

func (b *Backend) Capacity(context.Context) (map[string]int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return maps.Clone(b.capacity), nil
}

func (b *Backend) Apply(_ context.Context, objs []cluster.Manifest) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, o := range objs {
		if err := cluster.CheckJSONTypes(o); err != nil {
			return fmt.Errorf("fake apply %v: %w", o["kind"], err)
		}
		md, _ := o["metadata"].(map[string]any)
		labels, _ := md["labels"].(map[string]any)
		annotations, _ := md["annotations"].(map[string]any)
		idStr, _ := labels[cluster.LabelWorkloadID].(string)
		desired, ok := annotations[cluster.AnnotationDesiredRunning].(string)
		if idStr == "" || !ok {
			continue // tenant-level objects and secondary objects
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return fmt.Errorf("fake apply: bad workload id label: %w", err)
		}
		b.running[id] = desired == "true"
	}
	return nil
}

func (b *Backend) Observe(_ context.Context, _ string, id uuid.UUID, _ domain.WorkloadKind) (cluster.Observed, string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if o, ok := b.overrides[id]; ok {
		return o, "forced by test", nil
	}
	running, ok := b.running[id]
	switch {
	case !ok:
		return cluster.ObservedAbsent, "", nil
	case running:
		return cluster.ObservedReady, "", nil
	default:
		return cluster.ObservedStopped, "", nil
	}
}

func (b *Backend) Delete(_ context.Context, _ string, id uuid.UUID) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.running, id)
	delete(b.overrides, id)
	return nil
}
