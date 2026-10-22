package domain

// State is a workload's lifecycle state. The diagram lives in docs/architecture/control-plane.md;
// keep the two in sync.
type State string

const (
	StatePending      State = "pending"      // intent recorded; controller has not admitted it yet
	StateQueued       State = "queued"       // waiting for GPU capacity
	StateProvisioning State = "provisioning" // cluster objects created, not ready yet
	StateRunning      State = "running"
	StateStopping     State = "stopping"
	StateStopped      State = "stopped"
	StateFailed       State = "failed"
	StateDeleting     State = "deleting"
	StateDeleted      State = "deleted"
)

var transitions = map[State][]State{
	StatePending:      {StateQueued, StateProvisioning, StateDeleting, StateFailed},
	StateQueued:       {StateProvisioning, StateDeleting, StateFailed},
	StateProvisioning: {StateRunning, StateStopping, StateDeleting, StateFailed},
	StateRunning:      {StateStopping, StateDeleting, StateFailed},
	StateStopping:     {StateStopped, StateDeleting, StateFailed},
	StateStopped:      {StatePending, StateDeleting},
	StateFailed:       {StatePending, StateDeleting},
	StateDeleting:     {StateDeleted},
	StateDeleted:      {},
}

// AllStates lists every state in lifecycle order.
func AllStates() []State {
	return []State{
		StatePending, StateQueued, StateProvisioning, StateRunning, StateStopping,
		StateStopped, StateFailed, StateDeleting, StateDeleted,
	}
}

func (s State) Valid() bool {
	_, ok := transitions[s]
	return ok
}

// CanTransition reports whether from → to is a legal lifecycle step.
func CanTransition(from, to State) bool {
	for _, s := range transitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// HoldsQuota reports whether a workload in this state counts against its tenant's GPU quota.
func (s State) HoldsQuota() bool {
	switch s {
	case StatePending, StateQueued, StateProvisioning, StateRunning, StateStopping:
		return true
	}
	return false
}

// HoldsCapacity reports whether a workload in this state may have GPUs allocated in the
// cluster. The controller admits new work only into capacity not held by these.
func (s State) HoldsCapacity() bool {
	switch s {
	case StateProvisioning, StateRunning, StateStopping, StateDeleting:
		return true
	}
	return false
}

// Billable reports whether the customer pays for GPU time in this state.
// Provisioning time is not billed (ADR-0006).
func (s State) Billable() bool { return s == StateRunning }

// StatesWhere returns all states matching pred.
func StatesWhere(pred func(State) bool) []State {
	var out []State
	for _, s := range AllStates() {
		if pred(s) {
			out = append(out, s)
		}
	}
	return out
}
