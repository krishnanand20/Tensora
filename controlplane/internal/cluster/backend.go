// Package cluster is the only boundary between Tensora and Kubernetes (AGENTS.md rule 2).
// Implementations: fake (dev/tests) and kube (client-go, server-side apply).
package cluster

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/domain"
)

// Manifest is one Kubernetes object as unstructured JSON-compatible data.
type Manifest = map[string]any

// Observed is the runtime condition of a workload's cluster objects.
type Observed string

const (
	ObservedAbsent      Observed = "absent"
	ObservedProgressing Observed = "progressing"
	ObservedReady       Observed = "ready"
	ObservedStopped     Observed = "stopped"
	ObservedFailed      Observed = "failed"
)

type Backend interface {
	// Capacity returns schedulable GPUs per catalog GPU class across Ready, uncordoned nodes.
	// The controller subtracts what Postgres says is allocated.
	Capacity(ctx context.Context) (map[string]int, error)
	// Apply creates or updates objects (server-side apply). Order matters: namespaces first.
	Apply(ctx context.Context, objs []Manifest) error
	// Observe reports the state of a workload's primary object plus a human-readable reason. The
	// reason is shown to the workload's owner, so it must come from a fixed vocabulary: never raw
	// runtime messages, object or node names, or anything that echoes inputs (ADR-0015).
	Observe(ctx context.Context, namespace string, workloadID uuid.UUID, kind domain.WorkloadKind) (Observed, string, error)
	// Delete removes every object labeled with the workload ID, including its volumes. Idempotent.
	Delete(ctx context.Context, namespace string, workloadID uuid.UUID) error
}

// CheckJSONTypes reports values Kubernetes' unstructured machinery cannot handle: it accepts
// only map[string]any, []any, string, bool, int64, float64, and nil. Builders must pass this;
// the fake backend enforces it so mistakes surface in tests, not in production.
func CheckJSONTypes(v any) error { return checkJSON(v, "$") }

func checkJSON(v any, path string) error {
	switch x := v.(type) {
	case nil, string, bool, int64, float64:
		return nil
	case map[string]any:
		for k, e := range x {
			if err := checkJSON(e, path+"."+k); err != nil {
				return err
			}
		}
		return nil
	case []any:
		for i, e := range x {
			if err := checkJSON(e, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("%s: unsupported type %T (use int64, []any, map[string]any)", path, v)
}
