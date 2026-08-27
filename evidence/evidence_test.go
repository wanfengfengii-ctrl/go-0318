package evidence

import (
	"errors"
	"math"
	"testing"
)

func TestCompensatedPressureExact(t *testing.T) {
	// coeffPPM=10, ref=20000 mC; temp 20100 mC => delta=100.
	// term = 1000000 * 10 * 100 / 1_000_000 = 1000 Pa.
	got, err := CompensatedPressure(1_000_000, 20_100, CompensationParams{RefTempMC: 20_000, TempCoeffPPM: 10})
	if err != nil {
		t.Fatalf("CompensatedPressure: %v", err)
	}
	if got != 1_001_000 {
		t.Fatalf("CompensatedPressure = %d, want 1001000", got)
	}
}

func TestCompensatedPressureTruncatesTowardZero(t *testing.T) {
	// delta = 1 - 0 = 1; term = 500000 * 3 * 1 / 1_000_000 = 1.5 -> 1 (toward zero)
	got, err := CompensatedPressure(500_000, 1, CompensationParams{RefTempMC: 0, TempCoeffPPM: 3})
	if err != nil {
		t.Fatalf("CompensatedPressure: %v", err)
	}
	if got != 500_001 {
		t.Fatalf("CompensatedPressure = %d, want 500001", got)
	}
	// Negative truncation toward zero: -1.5 -> -1.
	got, err = CompensatedPressure(500_000, -1, CompensationParams{RefTempMC: 0, TempCoeffPPM: 3})
	if err != nil {
		t.Fatalf("CompensatedPressure: %v", err)
	}
	if got != 499_999 {
		t.Fatalf("CompensatedPressure = %d, want 499999", got)
	}
}

func TestRampRateExact(t *testing.T) {
	// dp=500000 Pa over 5000 ms -> 100000 Pa/s
	got, err := RampRate(1_000_000, 1_500_000, 0, 5_000)
	if err != nil {
		t.Fatalf("RampRate: %v", err)
	}
	if got != 100_000 {
		t.Fatalf("RampRate = %d, want 100000", got)
	}
}

func TestPressureDropExact(t *testing.T) {
	got, err := PressureDrop(10_000_000, 9_900_000)
	if err != nil {
		t.Fatalf("PressureDrop: %v", err)
	}
	if got != 100_000 {
		t.Fatalf("PressureDrop = %d, want 100000", got)
	}
}

func TestLeakRateExact(t *testing.T) {
	// volume=1000 uL, drop=100000 Pa, start=10000000 Pa, elapsed=1000 ms
	// leak = 1000 * 100000 * 1000 / (10000000 * 1000) = 10 uL/s
	got, err := LeakRate(10_000_000, 9_900_000, 1_000, 1_000)
	if err != nil {
		t.Fatalf("LeakRate: %v", err)
	}
	if got != 10 {
		t.Fatalf("LeakRate = %d, want 10", got)
	}
}

func TestMulDivDivideByZero(t *testing.T) {
	_, err := MulDiv(10, 10, 0)
	if !errors.Is(err, ErrDivideZero) {
		t.Fatalf("MulDiv by zero err = %v, want ErrDivideZero", err)
	}
}

func TestMulOverflow(t *testing.T) {
	_, err := Mul(math.MaxInt64, 2)
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("Mul overflow err = %v, want ErrOverflow", err)
	}
}

func TestAbsMinIntOverflow(t *testing.T) {
	_, err := Abs(math.MinInt64)
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("Abs(MinInt64) err = %v, want ErrOverflow", err)
	}
}

func TestValveDelayExact(t *testing.T) {
	got, err := ValveDelay(1_000, 1_250)
	if err != nil {
		t.Fatalf("ValveDelay: %v", err)
	}
	if got != 250 {
		t.Fatalf("ValveDelay = %d, want 250", got)
	}
}
