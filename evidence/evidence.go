// Package evidence implements the sampling and evidence engine: the append-only
// sample, valve receipt, and device-call records, and the checked integer
// arithmetic used to compute temperature-compensated pressure, ramp rates,
// pressure drop, leak rate, and valve response delay.
package evidence

import (
	"errors"
	"math"
)

// Errors returned by the checked arithmetic helpers.
var (
	ErrOverflow   = errors.New("int64 overflow")
	ErrDivideZero = errors.New("division by zero")
)

// Sample is an appended raw integer sample, ordered by a strictly increasing
// logical clock.
type Sample struct {
	TrialID    string `json:"trial_id"`
	Round      int    `json:"round"`
	Seq        int64  `json:"seq"`
	LogicalMs  int64  `json:"logical_ms"`
	PressurePa int64  `json:"pressure_pa"`
	TempMC     int64  `json:"temp_mc"`
	FlowULPerS int64  `json:"flow_ul_per_s"`
	ValvePos   int    `json:"valve_pos"`
}

// ValveReceipt is a device acknowledgement for a valve position change.
type ValveReceipt struct {
	TrialID   string `json:"trial_id"`
	Round     int    `json:"round"`
	Seq       int64  `json:"seq"`
	LogicalMs int64  `json:"logical_ms"`
	ValveID   string `json:"valve_id"`
	Position  int    `json:"position"`
	DelayMs   int64  `json:"delay_ms"`
}

// DeviceCall is a deterministic retry record written when a device fails.
// Device failures never produce qualified evidence, only a call record with a
// fixed retry sequence number and the next logical instant.
type DeviceCall struct {
	TrialID       string `json:"trial_id"`
	Round         int    `json:"round"`
	Seq           int64  `json:"seq"`
	LogicalMs     int64  `json:"logical_ms"`
	RetryNo       int    `json:"retry_no"`
	NextLogicalMs int64  `json:"next_logical_ms"`
	Kind          string `json:"kind"`
	Reason        string `json:"reason"`
}

// CompensationParams are the frozen temperature-compensation parameters.
type CompensationParams struct {
	RefTempMC    int64
	TempCoeffPPM int64
}

// Metrics holds the computed evidence values for a sample window.
type Metrics struct {
	CompensatedPressurePa int64 `json:"compensated_pressure_pa"`
	RampRatePaPerS        int64 `json:"ramp_rate_pa_per_s"`
	PressureDropPa        int64 `json:"pressure_drop_pa"`
	LeakRateULPerS        int64 `json:"leak_rate_ul_per_s"`
	ValveDelayMs          int64 `json:"valve_delay_ms"`
}

// Add returns a+b, checking for int64 overflow.
func Add(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, ErrOverflow
	}
	return a + b, nil
}

// Sub returns a-b, checking for int64 overflow.
func Sub(a, b int64) (int64, error) {
	if (b < 0 && a > math.MaxInt64+b) || (b > 0 && a < math.MinInt64+b) {
		return 0, ErrOverflow
	}
	return a - b, nil
}

// Mul returns a*b, checking for int64 overflow.
func Mul(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	r := a * b
	if r/b != a || (a == math.MinInt64 && b == -1) || (b == math.MinInt64 && a == -1) {
		return 0, ErrOverflow
	}
	return r, nil
}

// Abs returns the absolute value of a, checking for int64 overflow.
func Abs(a int64) (int64, error) {
	if a == math.MinInt64 {
		return 0, ErrOverflow
	}
	if a < 0 {
		return -a, nil
	}
	return a, nil
}

// MulDiv returns (a*b)/c truncated toward zero, checking both the multiply for
// overflow and the divide for zero. It matches the frozen integer formula used
// throughout the evidence engine.
func MulDiv(a, b, c int64) (int64, error) {
	if c == 0 {
		return 0, ErrDivideZero
	}
	p, err := Mul(a, b)
	if err != nil {
		return 0, err
	}
	return p / c, nil
}

// CompensatedPressure returns the temperature-compensated pressure using the
// frozen linear formula, truncating toward zero:
//
//	compensated = pressure + (pressure * coeffPPM * (temp - refTemp)) / 1_000_000
func CompensatedPressure(pressurePa, tempMC int64, p CompensationParams) (int64, error) {
	delta, err := Sub(tempMC, p.RefTempMC)
	if err != nil {
		return 0, err
	}
	term := pressurePa * p.TempCoeffPPM * delta / 1_000_000
	return Add(pressurePa, term)
}

// RampRate returns the pressure ramp rate in Pa/s over a time interval,
// truncating toward zero. The interval must be strictly positive.
func RampRate(pStartPa, pEndPa, tStartMs, tEndMs int64) (int64, error) {
	dt, err := Sub(tEndMs, tStartMs)
	if err != nil {
		return 0, err
	}
	if dt <= 0 {
		return 0, ErrDivideZero
	}
	dp, err := Sub(pEndPa, pStartPa)
	if err != nil {
		return 0, err
	}
	return MulDiv(dp, 1000, dt)
}

// PressureDrop returns the positive pressure fall from pStart to pEnd.
func PressureDrop(pStartPa, pEndPa int64) (int64, error) {
	drop, err := Sub(pStartPa, pEndPa)
	if err != nil {
		return 0, err
	}
	return Abs(drop)
}

// LeakRate estimates the leak rate in microlitres per second from a pressure
// decay using Boyle's law, truncating toward zero:
//
//	leak = volume * (pStart - pEnd) * 1000 / (pStart * elapsedMs)
func LeakRate(pStartPa, pEndPa, elapsedMs, volumeUL int64) (int64, error) {
	if elapsedMs <= 0 {
		return 0, ErrDivideZero
	}
	if pStartPa <= 0 {
		return 0, ErrDivideZero
	}
	drop, err := PressureDrop(pStartPa, pEndPa)
	if err != nil {
		return 0, err
	}
	num, err := Mul(drop, volumeUL)
	if err != nil {
		return 0, err
	}
	num, err = Mul(num, 1000)
	if err != nil {
		return 0, err
	}
	den, err := Mul(pStartPa, elapsedMs)
	if err != nil {
		return 0, err
	}
	return num / den, nil
}

// ValveDelay returns the valve response delay in milliseconds derived from a
// receipt's logical instant and the commanded instant.
func ValveDelay(commandMs, receiptMs int64) (int64, error) {
	return Sub(receiptMs, commandMs)
}
