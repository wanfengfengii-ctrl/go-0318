package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"abyssal-pressure-housing-qualification/evidence"
	"abyssal-pressure-housing-qualification/qualification"
)

func TestRetestOverpressurePropagatesToChamber(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)

	rs, err := svc.ReportAnomaly(context.Background(), ReportAnomalyRequest{
		TrialID: tr.ID, Kind: "overpressure", PortID: "p-sensor",
	})
	if err != nil {
		t.Fatalf("report anomaly: %v", err)
	}
	if len(rs.Members) == 0 {
		t.Fatal("expected retest members")
	}
	for _, m := range rs.Members {
		if m.Chamber != "c-main" {
			t.Fatalf("member chamber = %s, want c-main", m.Chamber)
		}
		if m.CheckType != "压力检查" {
			t.Fatalf("member check type = %s, want 压力检查", m.CheckType)
		}
	}
}

func TestRetestSealLeakPropagatesToBoundary(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)

	rs, err := svc.ReportAnomaly(context.Background(), ReportAnomalyRequest{
		TrialID: tr.ID, Kind: "seal_leak", SealID: "s-1",
	})
	if err != nil {
		t.Fatalf("report anomaly: %v", err)
	}
	if len(rs.Members) != 2 {
		t.Fatalf("expected 2 seal checks, got %d", len(rs.Members))
	}
	seen := map[string]bool{}
	for _, m := range rs.Members {
		seen[m.CheckType] = true
		if m.Chamber != "c-main" {
			t.Fatalf("seal member chamber = %s", m.Chamber)
		}
	}
	if !seen["外观检查"] || !seen["密封复查"] {
		t.Fatalf("expected both seal checks, got %v", seen)
	}
}

func TestRetestValveMismatchFollowsSharedPiping(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)

	rs, err := svc.ReportAnomaly(context.Background(), ReportAnomalyRequest{
		TrialID: tr.ID, Kind: "valve_mismatch", PortID: "p-sensor",
	})
	if err != nil {
		t.Fatalf("report anomaly: %v", err)
	}
	chambers := map[string]bool{}
	for _, m := range rs.Members {
		chambers[m.Chamber] = true
	}
	if !chambers["c-main"] || !chambers["c-end"] {
		t.Fatalf("expected propagation across shared piping to c-main and c-end, got %v", chambers)
	}
}

func TestConcurrentDuplicateReportSingleSet(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = svc.ReportAnomaly(context.Background(), ReportAnomalyRequest{
				TrialID: tr.ID, Kind: "overpressure", PortID: "p-sensor",
			})
		}(i)
	}
	close(start)
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			t.Fatalf("report anomaly: %v", e)
		}
	}
	rs, err := svc.GetRetestSet(context.Background(), tr.ID, 1)
	if err != nil {
		t.Fatalf("get retest set: %v", err)
	}
	// Members must be de-duplicated: each c-main port appears once.
	count := map[string]int{}
	for _, m := range rs.Members {
		count[m.PortID]++
	}
	for pid, n := range count {
		if n > 1 {
			t.Fatalf("port %s duplicated %d times", pid, n)
		}
	}
	if len(rs.Members) == 0 {
		t.Fatal("expected a retest set")
	}
}

func TestLateSampleRejectedAsRoundStale(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)

	if _, err := svc.RestartRound(context.Background(), RestartRoundRequest{TrialID: tr.ID}); err != nil {
		t.Fatalf("restart round: %v", err)
	}
	err := svc.SubmitSample(context.Background(), SubmitSampleRequest{
		TrialID: tr.ID, Round: 1, LogicalMs: 5000, PressurePa: 5_000_000, TempMC: 20_000,
	})
	if !errors.Is(err, evidence.ErrRoundStale) {
		t.Fatalf("expected round stale, got %v", err)
	}
}

var _ = qualification.AnomalyOverpressure
