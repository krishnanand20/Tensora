package service

import (
	"context"
	"time"

	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/store"
)

// Transition is the only way a workload changes state (AGENTS.md rule 7). It validates
// the step, compare-and-swaps the row, and appends the event metering bills from.
// On success w reflects the new state.
func Transition(ctx context.Context, q store.Queries, w *domain.Workload, to domain.State, reason string, at time.Time) error {
	if !domain.CanTransition(w.State, to) {
		return domain.Errorf(domain.CodeInvalidTransition, "cannot go from %s to %s", w.State, to)
	}
	at = at.UTC()
	if err := q.UpdateWorkloadState(ctx, w.ID, w.State, to, at); err != nil {
		return err
	}
	if err := q.AppendEvent(ctx, domain.Event{WorkloadID: w.ID, From: w.State, To: to, Reason: reason, At: at}); err != nil {
		return err
	}
	w.State, w.UpdatedAt = to, at
	return nil
}
