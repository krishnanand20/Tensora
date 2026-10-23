package metering

import (
	"context"
	"log/slog"
	"time"

	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/store"
)

const (
	// settleDelay gives in-flight transitions time to commit before an hour is closed.
	settleDelay = 5 * time.Minute
	// lookback bounds each pass; hours older than this are assumed final.
	lookback = 48 * time.Hour
)

type Meter struct {
	store store.Store
	log   *slog.Logger
	now   func() time.Time
}

func New(st store.Store, log *slog.Logger, now func() time.Time) *Meter {
	if now == nil {
		now = time.Now
	}
	return &Meter{store: st, log: log, now: now}
}

func (m *Meter) Run(ctx context.Context, interval time.Duration) error {
	m.log.Info("metering started", "interval", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if n, err := m.MeterOnce(ctx); err != nil {
			m.log.Error("metering pass failed", "err", err)
		} else if n > 0 {
			m.log.Info("usage records written", "count", n)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// MeterOnce writes a usage record for every workload-hour that has closed and has no
// record yet. Records are never updated, so running it repeatedly is safe.
// TODO(phase-3): batch by workload instead of one transaction once volume warrants it.
func (m *Meter) MeterOnce(ctx context.Context) (int, error) {
	closed := m.now().UTC().Add(-settleDelay).Truncate(time.Hour)
	since := closed.Add(-lookback)
	written := 0
	err := m.store.InTx(ctx, func(q store.Queries) error {
		ws, err := q.WorkloadsForMetering(ctx, since)
		if err != nil {
			return err
		}
		for _, w := range ws {
			events, err := q.Events(ctx, w.ID)
			if err != nil {
				return err
			}
			for hour, secs := range SplitHourly(MeteredIntervals(events, closed)) {
				if hour.Before(since) || secs == 0 {
					continue
				}
				ok, err := q.InsertUsageIfAbsent(ctx, domain.UsageRecord{
					WorkloadID: w.ID, PeriodStart: hour, TenantID: w.TenantID,
					GPUClass: w.GPUClass, GPUCount: w.GPUCount, Seconds: secs,
				})
				if err != nil {
					return err
				}
				if ok {
					written++
				}
			}
		}
		return nil
	})
	return written, err
}
