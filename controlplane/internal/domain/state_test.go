package domain

import "testing"

func TestTransitions(t *testing.T) {
	tests := []struct {
		from, to State
		want     bool
	}{
		{StatePending, StateProvisioning, true},
		{StatePending, StateQueued, true},
		{StateQueued, StateProvisioning, true},
		{StateProvisioning, StateRunning, true},
		{StateRunning, StateStopping, true},
		{StateStopping, StateStopped, true},
		{StateStopped, StatePending, true},
		{StateFailed, StatePending, true},
		{StateDeleting, StateDeleted, true},

		{StatePending, StateRunning, false},
		{StateRunning, StatePending, false},
		{StateStopped, StateRunning, false},
		{StateDeleted, StateRunning, false},
		{StateDeleting, StateRunning, false},
	}
	for _, tt := range tests {
		if got := CanTransition(tt.from, tt.to); got != tt.want {
			t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestEveryStateIsDefinedAndOnlyDeletedIsTerminal(t *testing.T) {
	for _, s := range AllStates() {
		if !s.Valid() {
			t.Errorf("%s missing from transition table", s)
		}
		terminal := len(transitions[s]) == 0
		if terminal != (s == StateDeleted) {
			t.Errorf("%s: terminal=%v", s, terminal)
		}
	}
}

func TestEveryStateCanReachDeleted(t *testing.T) {
	for _, start := range AllStates() {
		seen := map[State]bool{start: true}
		queue := []State{start}
		for len(queue) > 0 {
			s := queue[0]
			queue = queue[1:]
			for _, n := range transitions[s] {
				if !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
		if !seen[StateDeleted] {
			t.Errorf("%s cannot reach deleted: a workload could be stuck forever", start)
		}
	}
}

func TestMeteredStatesHoldCapacity(t *testing.T) {
	// Counting usage for GPUs the scheduler thinks are free would be a correctness bug.
	for _, s := range AllStates() {
		if s.Metered() && !s.HoldsCapacity() {
			t.Errorf("%s is metered but does not hold capacity", s)
		}
	}
}
