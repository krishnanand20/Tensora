// Package metering turns workload lifecycle events into hourly usage records.
// Billing is based on allocation (time in a billable state), not GPU utilization (ADR-0006).
package metering

import (
	"time"

	"github.com/tensora/tensora/controlplane/internal/domain"
)

type Interval struct{ Start, End time.Time }

// BillableIntervals turns a workload's time-ordered events into the periods it spent in a
// billable state, clipped to until. An interval still open at until ends there.
func BillableIntervals(events []domain.Event, until time.Time) []Interval {
	var out []Interval
	var start time.Time
	open := false
	for _, e := range events {
		if !e.At.Before(until) {
			break
		}
		switch billable := e.To.Billable(); {
		case billable && !open:
			start, open = e.At, true
		case !billable && open:
			out = append(out, Interval{start, e.At})
			open = false
		}
	}
	if open && start.Before(until) {
		out = append(out, Interval{start, until})
	}
	return out
}

// SplitHourly returns whole billable seconds per UTC clock hour.
func SplitHourly(intervals []Interval) map[time.Time]int64 {
	out := map[time.Time]int64{}
	for _, iv := range intervals {
		end := iv.End.UTC()
		for t := iv.Start.UTC(); t.Before(end); {
			hour := t.Truncate(time.Hour)
			next := hour.Add(time.Hour)
			if end.Before(next) {
				next = end
			}
			out[hour] += int64(next.Sub(t) / time.Second)
			t = next
		}
	}
	return out
}
