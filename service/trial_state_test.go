package service

import (
	"context"
	"errors"
	"testing"

	"abyssal-pressure-housing-qualification/evidence"
	"abyssal-pressure-housing-qualification/trial"
)

func TestLegalStageAndStepProgression(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)

	for _, st := range []string{"fill_vent", "step_ramp"} {
		updated, err := svc.AdvanceStage(context.Background(), StageRequest{TrialID: tr.ID, Stage: st})
		if err != nil {
			t.Fatalf("advance to %s: %v", st, err)
		}
		tr = updated
	}

	submitStableSamples(t, svc, tr.ID, 2000, 5)
	updated, err := svc.CompleteStep(context.Background(), CompleteStepRequest{
		TrialID: tr.ID, StepIndex: 1, StartMs: 2000, EndMs: 2006,
	})
	if err != nil {
		t.Fatalf("complete step 1: %v", err)
	}
	if updated.StepIndex != 2 {
		t.Fatalf("step index = %d, want 2", updated.StepIndex)
	}
	if updated.Stage != trial.StageStepRamp {
		t.Fatalf("stage = %s, want step_ramp", updated.Stage)
	}
}

func TestAdvanceStageRejectsSkip(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)
	_, err := svc.AdvanceStage(context.Background(), StageRequest{TrialID: tr.ID, Stage: "step_ramp"})
	if err == nil {
		t.Fatal("expected skipping fill_vent to be rejected")
	}
	var te *trial.StageTransitionError
	if !errors.As(err, &te) {
		t.Fatalf("expected StageTransitionError, got %T", err)
	}
}

func TestSampleRejectedWhenLeaseExactlyExpired(t *testing.T) {
	svc := newTestService(t)
	snap := freezeTestConfig(t, svc)
	tr := createTrial(t, svc, snap.Digest)

	if err := svc.Startup(context.Background(), StartupRequest{
		TrialID: tr.ID,
		Leases:  []trial.Lease{{ResourceID: "collector-1", ExpiresAt: 1100}},
	}); err != nil {
		t.Fatalf("startup: %v", err)
	}

	// Valid strictly before expiry.
	if err := svc.SubmitSample(context.Background(), SubmitSampleRequest{
		TrialID: tr.ID, LogicalMs: 1099, PressurePa: 5_000_000, TempMC: 20_000,
	}); err != nil {
		t.Fatalf("sample before expiry: %v", err)
	}
	// At exactly the expiry instant the sample is rejected.
	err := svc.SubmitSample(context.Background(), SubmitSampleRequest{
		TrialID: tr.ID, LogicalMs: 1100, PressurePa: 5_000_000, TempMC: 20_000,
	})
	if !errors.Is(err, evidence.ErrLeaseExpired) {
		t.Fatalf("expected lease expired, got %v", err)
	}
}

func TestOutOfOrderSampleRejected(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)

	if err := svc.SubmitSample(context.Background(), SubmitSampleRequest{
		TrialID: tr.ID, LogicalMs: 2000, PressurePa: 5_000_000, TempMC: 20_000,
	}); err != nil {
		t.Fatalf("first sample: %v", err)
	}
	err := svc.SubmitSample(context.Background(), SubmitSampleRequest{
		TrialID: tr.ID, LogicalMs: 1999, PressurePa: 5_000_000, TempMC: 20_000,
	})
	if !errors.Is(err, evidence.ErrSampleOutOfOrder) {
		t.Fatalf("expected sample out of order, got %v", err)
	}
	samples, err := svc.ListSamples(context.Background(), tr.ID, 1)
	if err != nil {
		t.Fatalf("list samples: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample in evidence chain, got %d", len(samples))
	}
}
