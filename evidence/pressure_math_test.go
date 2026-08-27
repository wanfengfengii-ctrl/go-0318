package evidence

import (
	"errors"
	"testing"
)

func stepParams() StepParams {
	return StepParams{
		TargetPa: 10_000_000, RampUpPaPerS: 100_000, RampDownPaPerS: 100_000,
		HoldMs: 600_000, LeakLimitULPerS: 10, MaxDropPa: 100_000,
		VolumeUL: 1_000_000, MaxPressurePa: 20_000_000,
	}
}

func refComp() CompensationParams { return CompensationParams{RefTempMC: 20_000, TempCoeffPPM: 10} }

func TestInWindowHalfOpenBoundary(t *testing.T) {
	if !InWindow(100, 100, 200) {
		t.Fatal("expected start boundary to be inclusive")
	}
	if InWindow(200, 100, 200) {
		t.Fatal("expected end boundary to be exclusive")
	}
	if !InWindow(199, 100, 200) {
		t.Fatal("expected 199 inside [100,200)")
	}
}

func TestEvaluateRejectsOverpressure(t *testing.T) {
	w := EvidenceWindow{TrialID: "t", Round: 1, StepIndex: 1, StartMs: 0, EndMs: 10, Samples: []Sample{
		{LogicalMs: 1, PressurePa: 21_000_000, TempMC: 20_000},
		{LogicalMs: 2, PressurePa: 21_000_000, TempMC: 20_000},
	}}
	_, err := Evaluate(w, stepParams(), refComp())
	if !errors.Is(err, ErrOverpressure) {
		t.Fatalf("expected overpressure, got %v", err)
	}
}

func TestEvaluateRejectsRateExceeded(t *testing.T) {
	w := EvidenceWindow{TrialID: "t", Round: 1, StepIndex: 1, StartMs: 0, EndMs: 10, Samples: []Sample{
		{LogicalMs: 1, PressurePa: 0, TempMC: 20_000},
		{LogicalMs: 2, PressurePa: 200_000, TempMC: 20_000},
	}}
	_, err := Evaluate(w, stepParams(), refComp())
	if !errors.Is(err, ErrRateExceeded) {
		t.Fatalf("expected rate exceeded, got %v", err)
	}
}

func TestEvaluateRejectsLeakLimit(t *testing.T) {
	p := stepParams()
	w := EvidenceWindow{TrialID: "t", Round: 1, StepIndex: 1, StartMs: 0, EndMs: 2000, Samples: []Sample{
		{LogicalMs: 1, PressurePa: 10_000_000, TempMC: 20_000},
		{LogicalMs: 1000, PressurePa: 9_950_000, TempMC: 20_000},
	}}
	_, err := Evaluate(w, p, refComp())
	if !errors.Is(err, ErrLeakLimit) {
		t.Fatalf("expected leak limit exceeded, got %v", err)
	}
}

func TestEvaluateRejectsEmptyWindow(t *testing.T) {
	w := EvidenceWindow{TrialID: "t", Round: 1, StepIndex: 1, StartMs: 0, EndMs: 10}
	_, err := Evaluate(w, stepParams(), refComp())
	if !errors.Is(err, ErrWindowMismatch) {
		t.Fatalf("expected window mismatch, got %v", err)
	}
}

func TestEvaluateQualifiesStableWindow(t *testing.T) {
	w := EvidenceWindow{TrialID: "t", Round: 1, StepIndex: 1, StartMs: 0, EndMs: 10, Samples: []Sample{
		{LogicalMs: 1, PressurePa: 5_000_000, TempMC: 20_000},
		{LogicalMs: 2, PressurePa: 5_000_000, TempMC: 20_000},
		{LogicalMs: 3, PressurePa: 5_000_000, TempMC: 20_000},
	}}
	m, err := Evaluate(w, stepParams(), refComp())
	if err != nil {
		t.Fatalf("expected stable window to qualify, got %v", err)
	}
	if m.RampRatePaPerS != 0 || m.PressureDropPa != 0 || m.LeakRateULPerS != 0 {
		t.Fatalf("unexpected metrics for constant window: %+v", m)
	}
}
