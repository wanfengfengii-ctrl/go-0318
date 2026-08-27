package service

import (
	"context"
	"testing"

	"abyssal-pressure-housing-qualification/trial"
)

func TestDeviceFailuresProduceIncrementingRetries(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)

	// Timeout: device reports an error.
	if err := svc.SubmitDeviceResult(context.Background(), SubmitDeviceResultRequest{
		TrialID: tr.ID, LogicalMs: 2000, Kind: "device", Error: "timeout",
	}); err != nil {
		t.Fatalf("timeout result: %v", err)
	}
	// Format error: unknown kind.
	if err := svc.SubmitDeviceResult(context.Background(), SubmitDeviceResultRequest{
		TrialID: tr.ID, LogicalMs: 2100, Kind: "bogus",
	}); err != nil {
		t.Fatalf("format error result: %v", err)
	}
	// Valve mismatch: reported position differs from commanded.
	if err := svc.SubmitDeviceResult(context.Background(), SubmitDeviceResultRequest{
		TrialID: tr.ID, LogicalMs: 2200, Kind: "valve", ValveID: "v-1",
		CommandedPos: 1, ValvePos: 2, CommandedMs: 2200,
	}); err != nil {
		t.Fatalf("valve mismatch result: %v", err)
	}

	calls, err := svc.ListDeviceCalls(context.Background(), tr.ID, 1)
	if err != nil {
		t.Fatalf("list device calls: %v", err)
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 device calls, got %d", len(calls))
	}
	for i, c := range calls {
		if c.RetryNo != i+1 {
			t.Fatalf("retry %d has number %d, want %d", i, c.RetryNo, i+1)
		}
	}
	// No qualified pressure or valve evidence may have been produced.
	receipts, _ := svc.ListValveReceipts(context.Background(), tr.ID, 1)
	if len(receipts) != 0 {
		t.Fatalf("expected no valve receipts, got %d", len(receipts))
	}
	samples, _ := svc.ListSamples(context.Background(), tr.ID, 1)
	if len(samples) != 0 {
		t.Fatalf("expected no samples, got %d", len(samples))
	}
}

func TestCalibrationStaleProducesRetry(t *testing.T) {
	svc := newTestService(t)
	cfg := testConfig()
	cfg.Calibrations[0].ExpiresAtMs = 500 // long past the injected clock
	snap, err := svc.FreezeConfiguration(context.Background(), cfg)
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	tr := createTrial(t, svc, snap.Digest)
	if err := svc.Startup(context.Background(), StartupRequest{
		TrialID: tr.ID, Leases: []trial.Lease{{ResourceID: "collector-1"}},
	}); err != nil {
		t.Fatalf("startup: %v", err)
	}

	if err := svc.SubmitDeviceResult(context.Background(), SubmitDeviceResultRequest{
		TrialID: tr.ID, LogicalMs: 2000, Kind: "sample", Channel: "ch-1",
	}); err != nil {
		t.Fatalf("stale calibration result: %v", err)
	}
	calls, err := svc.ListDeviceCalls(context.Background(), tr.ID, 1)
	if err != nil {
		t.Fatalf("list device calls: %v", err)
	}
	if len(calls) != 1 || calls[0].Reason != "calibration_stale" {
		t.Fatalf("expected one calibration_stale call, got %+v", calls)
	}
}

func TestStableWindowProducesSingleStepEvidence(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)
	submitStableSamples(t, svc, tr.ID, 2000, 5)

	if _, err := svc.CompleteStep(context.Background(), CompleteStepRequest{
		TrialID: tr.ID, StepIndex: 1, StartMs: 2000, EndMs: 2006,
	}); err != nil {
		t.Fatalf("complete step: %v", err)
	}
	windows, err := svc.ListWindows(context.Background(), tr.ID, 1)
	if err != nil {
		t.Fatalf("list windows: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("expected exactly one evidence window, got %d", len(windows))
	}
	if windows[0].StepIndex != 1 {
		t.Fatalf("window step index = %d, want 1", windows[0].StepIndex)
	}
	// Completing the same step again is out of order.
	if _, err := svc.CompleteStep(context.Background(), CompleteStepRequest{
		TrialID: tr.ID, StepIndex: 1, StartMs: 2000, EndMs: 2006,
	}); err == nil {
		t.Fatal("expected re-completing step 1 to fail")
	}
}
