package metering_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/metering"
	"github.com/tensora/tensora/controlplane/internal/service"
	"github.com/tensora/tensora/controlplane/internal/store"
	"github.com/tensora/tensora/controlplane/internal/testutil"
)

var base = time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

func at(minutes int) time.Time { return base.Add(time.Duration(minutes) * time.Minute) }

func ev(minutes int, to domain.State) domain.Event { return domain.Event{To: to, At: at(minutes)} }

func TestMeteredIntervals(t *testing.T) {
	events := []domain.Event{
		ev(0, domain.StatePending), ev(1, domain.StateProvisioning), ev(2, domain.StateRunning),
		ev(62, domain.StateStopping), ev(63, domain.StateStopped),
		ev(100, domain.StatePending), ev(101, domain.StateProvisioning), ev(102, domain.StateRunning),
	}
	got := metering.MeteredIntervals(events, at(130))
	want := []metering.Interval{{at(2), at(62)}, {at(102), at(130)}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if !got[i].Start.Equal(want[i].Start) || !got[i].End.Equal(want[i].End) {
			t.Errorf("interval %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestEventsAfterUntilAreIgnored(t *testing.T) {
	got := metering.MeteredIntervals([]domain.Event{ev(70, domain.StateRunning)}, at(60))
	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestSplitHourly(t *testing.T) {
	got := metering.SplitHourly([]metering.Interval{{at(2), at(62)}, {at(102), at(130)}})
	want := map[time.Time]int64{
		at(0):   58 * 60,      // 10:02–11:00
		at(60):  2*60 + 18*60, // 11:00–11:02 and 11:42–12:00
		at(120): 10 * 60,      // 12:00–12:10
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for hour, secs := range want {
		if got[hour] != secs {
			t.Errorf("%s: got %d, want %d", hour.Format("15:04"), got[hour], secs)
		}
	}
}

func TestMeterOnceBillsClosedHoursExactlyOnce(t *testing.T) {
	st := testutil.SeededStore(t)
	ctx := context.Background()
	var tenantID uuid.UUID
	w := domain.Workload{
		ID: uuid.New(), Name: "w", Kind: domain.KindNotebook, State: domain.StatePending, ImageID: "pytorch-notebook",
		GPUClass: "l40s-48gb", GPUCount: 2, Params: map[string]string{}, CreatedBy: "t", CreatedAt: at(0), UpdatedAt: at(0),
	}
	err := st.InTx(ctx, func(q store.Queries) error {
		tn, err := q.TenantBySlug(ctx, "acme")
		if err != nil {
			return err
		}
		tenantID, w.TenantID = tn.ID, tn.ID
		if err := q.CreateWorkload(ctx, w); err != nil {
			return err
		}
		steps := []struct {
			min int
			to  domain.State
		}{{1, domain.StateProvisioning}, {2, domain.StateRunning}, {62, domain.StateStopping}, {63, domain.StateStopped}}
		for _, s := range steps {
			if err := service.Transition(ctx, q, &w, s.to, "test", at(s.min)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	now := at(150) // 12:30 → hours 10:00 and 11:00 are closed
	m := metering.New(st, slog.New(slog.DiscardHandler), func() time.Time { return now })
	for pass, wantWritten := range []int{2, 0} {
		n, err := m.MeterOnce(ctx)
		if err != nil || n != wantWritten {
			t.Fatalf("pass %d: wrote %d, %v; want %d", pass, n, err, wantWritten)
		}
	}
	_ = st.InTx(ctx, func(q store.Queries) error {
		recs, _ := q.Usage(ctx, tenantID, base, base.Add(24*time.Hour))
		if len(recs) != 2 || recs[0].Seconds != 58*60 || recs[1].Seconds != 2*60 || recs[0].GPUSeconds() != 2*58*60 {
			t.Errorf("records = %+v", recs)
		}
		return nil
	})
}
