package service

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"abyssal-pressure-housing-qualification/qualification"
	"abyssal-pressure-housing-qualification/store"
)

// simultaneousRetestReads makes the first two reports observe the same absent
// retest-set snapshot. This deterministically exercises the lost-update edge
// without depending on goroutine scheduling.
type simultaneousRetestReads struct {
	store.Store
	mu      sync.Mutex
	reads   int
	arrived chan struct{}
}

func (s *simultaneousRetestReads) GetRetestSet(ctx context.Context, trialID string, round int) (*qualification.RetestSet, error) {
	s.mu.Lock()
	s.reads++
	read := s.reads
	if read == 2 {
		close(s.arrived)
	}
	s.mu.Unlock()
	if read <= 2 {
		<-s.arrived
		return nil, store.ErrNotFound
	}
	return s.Store.GetRetestSet(ctx, trialID, round)
}

func TestModel_ConcurrentAnomaliesMergeRetestScope(t *testing.T) {
	cases := []struct {
		name string
		reqs [2]ReportAnomalyRequest
		want []qualification.RetestMember
	}{
		{
			name: "distinct anomaly propagation is merged and canonically sorted",
			reqs: [2]ReportAnomalyRequest{
				{Kind: "overpressure", PortID: "p-sensor"},
				{Kind: "seal_leak", SealID: "s-1"},
			},
			want: []qualification.RetestMember{
				{Chamber: "c-main", CheckType: "外观检查"},
				{Chamber: "c-main", CheckType: "密封复查"},
				{Chamber: "c-main", PortID: "p-inlet", CheckType: "压力检查"},
				{Chamber: "c-main", PortID: "p-sensor", CheckType: "压力检查"},
			},
		},
		{
			name: "duplicate anomaly propagation has one copy of each member",
			reqs: [2]ReportAnomalyRequest{
				{Kind: "overpressure", PortID: "p-sensor"},
				{Kind: "overpressure", PortID: "p-sensor"},
			},
			want: []qualification.RetestMember{
				{Chamber: "c-main", PortID: "p-inlet", CheckType: "压力检查"},
				{Chamber: "c-main", PortID: "p-sensor", CheckType: "压力检查"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := newTestService(t)
			_, tr := startTrial(t, base)
			gated := &simultaneousRetestReads{Store: base.store, arrived: make(chan struct{})}
			svc := New(gated)
			svc.SetClock(func() int64 { return testNow })

			start := make(chan struct{})
			errs := make([]error, len(tc.reqs))
			var wg sync.WaitGroup
			for i := range tc.reqs {
				tc.reqs[i].TrialID = tr.ID
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					_, errs[i] = svc.ReportAnomaly(context.Background(), tc.reqs[i])
				}(i)
			}
			close(start)
			wg.Wait()
			for i, err := range errs {
				if err != nil {
					t.Fatalf("report anomaly %d: %v", i, err)
				}
			}

			got, err := base.GetRetestSet(context.Background(), tr.ID, tr.Round)
			if err != nil {
				t.Fatalf("get retest set: %v", err)
			}
			if got == nil || !reflect.DeepEqual(got.Members, tc.want) {
				t.Fatalf("retest members = %#v, want %#v", got, tc.want)
			}

			if err := base.ClearRetestSet(context.Background(), tr.ID, tr.Round); err != nil {
				t.Fatalf("clear retest set: %v", err)
			}
			cleared, err := base.GetRetestSet(context.Background(), tr.ID, tr.Round)
			if err != nil {
				t.Fatalf("get cleared retest set: %v", err)
			}
			if cleared == nil || len(cleared.Members) != 0 {
				t.Fatalf("cleared retest set retained members: %#v", cleared)
			}
		})
	}
}
