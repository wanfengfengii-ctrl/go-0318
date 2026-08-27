package service

import (
	"context"
	"testing"

	"abyssal-pressure-housing-qualification/configuration"
	"abyssal-pressure-housing-qualification/store"
	"abyssal-pressure-housing-qualification/trial"
)

// newTestService opens an in-memory store and returns a service with a
// deterministic injected clock.
func newTestService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	svc := New(st)
	svc.SetClock(func() int64 { return testNow })
	return svc
}

// testNow is the injected logical clock, advanced by tests.
var testNow int64 = 1000

func testConfig() configuration.Input {
	return configuration.Input{
		Chambers: []configuration.ChamberSection{
			{ID: "c-main", Name: "主承压舱段", VolumeUL: 1000},
			{ID: "c-end", Name: "端盖舱段", VolumeUL: 500},
		},
		Ports: []configuration.Port{
			{ID: "p-inlet", Chamber: "c-main", Kind: configuration.PortPressureInlet},
			{ID: "p-sensor", Chamber: "c-main", Kind: configuration.PortPressureSensor, Channel: "ch-1"},
			{ID: "p-temp", Chamber: "c-end", Kind: configuration.PortTemperatureSensor, Channel: "ch-2"},
		},
		Pipes: []configuration.Pipe{{ID: "pipe-1", From: "p-sensor", To: "p-temp"}},
		SealBoundaries: []configuration.SealBoundary{
			{ID: "s-1", Chamber: "c-main", Checks: []string{"外观检查", "密封复查"}},
		},
		Steps: []configuration.PressureStep{
			{Index: 1, TargetPa: 5_000_000, RampUpPaPerS: 100_000, RampDownPaPerS: 100_000, HoldMs: 600_000, LeakLimitULPerS: 10, MaxDropPa: 50_000},
			{Index: 2, TargetPa: 10_000_000, RampUpPaPerS: 100_000, RampDownPaPerS: 100_000, HoldMs: 600_000, LeakLimitULPerS: 10, MaxDropPa: 50_000},
		},
		Calibrations: []configuration.Calibration{
			{Channel: "ch-1", Serial: "SN-P", ExpiresAtMs: 2_000_000_000_000, Summary: "压力"},
			{Channel: "ch-2", Serial: "SN-T", ExpiresAtMs: 2_000_000_000_000, Summary: "温度"},
		},
		Compensation: configuration.CompensationParams{RefTempMC: 20_000, TempCoeffPPM: 10},
	}
}

func freezeTestConfig(t *testing.T, svc *Service) *configuration.Snapshot {
	t.Helper()
	snap, err := svc.FreezeConfiguration(context.Background(), testConfig())
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	return snap
}

func createTrial(t *testing.T, svc *Service, digest string) *trial.Trial {
	t.Helper()
	tr, err := svc.CreateTrial(context.Background(), CreateTrialRequest{ConfigDigest: digest})
	if err != nil {
		t.Fatalf("create trial: %v", err)
	}
	return tr
}

// startTrial freezes a config, creates a trial, and performs an atomic startup
// with four resource leases valid far into the future.
func startTrial(t *testing.T, svc *Service) (*configuration.Snapshot, *trial.Trial) {
	t.Helper()
	snap := freezeTestConfig(t, svc)
	tr := createTrial(t, svc, snap.Digest)
	err := svc.Startup(context.Background(), StartupRequest{
		TrialID: tr.ID,
		Bindings: []trial.Binding{
			{Serial: "SN-P", Type: trial.ComponentPressureSensor, Position: "p-sensor"},
			{Serial: "SN-T", Type: trial.ComponentTemperatureSensor, Position: "p-temp"},
		},
		Leases: []trial.Lease{
			{ResourceID: "chamber-1"},
			{ResourceID: "pump-1"},
			{ResourceID: "collector-1"},
			{ResourceID: "valve-1"},
		},
	})
	if err != nil {
		t.Fatalf("startup: %v", err)
	}
	return snap, tr
}

// submitStableSamples appends n constant-pressure samples at increasing logical
// times, forming a window that satisfies every threshold.
func submitStableSamples(t *testing.T, svc *Service, trialID string, startMs int64, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		ts := startMs + int64(i)
		err := svc.SubmitSample(context.Background(), SubmitSampleRequest{
			TrialID: trialID, LogicalMs: ts, PressurePa: 5_000_000, TempMC: 20_000,
		})
		if err != nil {
			t.Fatalf("submit sample %d: %v", i, err)
		}
	}
}
