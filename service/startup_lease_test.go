package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"abyssal-pressure-housing-qualification/trial"
)

func TestStartupBindsAndLeases(t *testing.T) {
	svc := newTestService(t)
	snap, tr := startTrial(t, svc)

	leases, err := svc.ListLeases(context.Background(), tr.ID, 1)
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 4 {
		t.Fatalf("expected 4 leases, got %d", len(leases))
	}
	bindings, err := svc.ListBindings(context.Background(), tr.ID, 1)
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(bindings))
	}
	if snap.Digest != tr.ConfigDigest {
		t.Fatalf("trial config digest mismatch")
	}
}

func TestConcurrentStartupSingleWinner(t *testing.T) {
	svc := newTestService(t)
	snap := freezeTestConfig(t, svc)
	t1 := createTrial(t, svc, snap.Digest)
	t2 := createTrial(t, svc, snap.Digest)

	start := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, id := range []string{t1.ID, t2.ID} {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			<-start
			errs[i] = svc.Startup(context.Background(), StartupRequest{
				TrialID:  id,
				Bindings: []trial.Binding{{Serial: "SN-" + id, Type: trial.ComponentPump, Position: "p-" + id}},
				Leases:   []trial.Lease{{ResourceID: "pump-1"}},
			})
		}(i, id)
	}
	close(start)
	wg.Wait()

	winners := 0
	for _, e := range errs {
		if e == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly one winner, got %d (errs=%v)", winners, errs)
	}

	// The loser must not have left any binding behind.
	for _, id := range []string{t1.ID, t2.ID} {
		bindings, err := svc.ListBindings(context.Background(), id, 1)
		if err != nil {
			t.Fatalf("list bindings: %v", err)
		}
		if len(bindings) > 1 {
			t.Fatalf("loser left bindings behind: %d", len(bindings))
		}
	}
}

func TestStartupIdempotentReplay(t *testing.T) {
	svc := newTestService(t)
	snap := freezeTestConfig(t, svc)
	tr := createTrial(t, svc, snap.Digest)

	req := StartupRequest{
		TrialID: tr.ID, OpNo: "op-1",
		Leases: []trial.Lease{{ResourceID: "pump-1"}},
	}
	if err := svc.Startup(context.Background(), req); err != nil {
		t.Fatalf("first startup: %v", err)
	}
	// Identical retry returns the original (nil) result without re-applying.
	if err := svc.Startup(context.Background(), req); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
}

func TestStartupIdempotencyConflict(t *testing.T) {
	svc := newTestService(t)
	snap := freezeTestConfig(t, svc)
	tr := createTrial(t, svc, snap.Digest)

	req1 := StartupRequest{TrialID: tr.ID, OpNo: "op-1", Leases: []trial.Lease{{ResourceID: "pump-1"}}}
	if err := svc.Startup(context.Background(), req1); err != nil {
		t.Fatalf("startup: %v", err)
	}
	req2 := StartupRequest{TrialID: tr.ID, OpNo: "op-1", Leases: []trial.Lease{{ResourceID: "pump-2"}}}
	err := svc.Startup(context.Background(), req2)
	if !errors.Is(err, trial.ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}
