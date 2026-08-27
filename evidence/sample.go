package evidence

import (
	"errors"
	"fmt"
)

// Sentinel errors surfaced as stable API error codes by the HTTP layer.
var (
	ErrSampleOutOfOrder = errors.New("sample out of order")
	ErrOverpressure     = errors.New("overpressure")
	ErrRateExceeded     = errors.New("ramp rate exceeded")
	ErrLeakLimit        = errors.New("leak limit exceeded")
	ErrDropExceeded     = errors.New("pressure drop exceeded")
	ErrCalibrationStale = errors.New("calibration stale")
	ErrLeaseExpired     = errors.New("lease expired")
	ErrRoundStale       = errors.New("round stale")
	ErrWindowMismatch   = errors.New("sample outside frozen window")
	ErrDeviceFailure    = errors.New("device failure")
	ErrValveMismatch    = errors.New("valve position mismatch")
)

// InWindow reports whether a logical instant lies within the half-open sampling
// window [start, end): the start boundary is inclusive and the end boundary is
// exclusive, so a sample at exactly the end instant belongs to the next window.
func InWindow(logicalMs, start, end int64) bool {
	return logicalMs >= start && logicalMs < end
}

// ValidateSampleOrder enforces the strictly increasing logical-clock invariant.
// Out-of-order or duplicate samples are rejected and never enter the evidence
// chain.
func ValidateSampleOrder(prevLogicalMs, nextLogicalMs int64) error {
	if nextLogicalMs <= prevLogicalMs {
		return fmt.Errorf("%w: logical time %d does not follow %d", ErrSampleOutOfOrder, nextLogicalMs, prevLogicalMs)
	}
	return nil
}

// ValidateCalibration checks a frozen calibration against a logical instant.
// A calibration is valid strictly before its expiry instant.
func ValidateCalibration(expiresAtMs, nowMs int64) error {
	if nowMs >= expiresAtMs {
		return fmt.Errorf("%w: expired at %d, now %d", ErrCalibrationStale, expiresAtMs, nowMs)
	}
	return nil
}

// ValidateLease checks that a lease is held by the expected owner, for the
// expected round, and has not expired at the given logical instant.
func ValidateLease(round, leaseRound int, holder, leaseHolder string, expiresAt, nowMs int64) error {
	if round != leaseRound {
		return fmt.Errorf("%w: round %d vs lease round %d", ErrRoundStale, round, leaseRound)
	}
	if holder != leaseHolder {
		return fmt.Errorf("%w: holder %q vs lease holder %q", ErrLeaseExpired, holder, leaseHolder)
	}
	if nowMs >= expiresAt {
		return fmt.Errorf("%w: expired at %d, now %d", ErrLeaseExpired, expiresAt, nowMs)
	}
	return nil
}

// NextRetryClock deterministically derives the next logical instant for a
// device retry: one tick after the current instant. Retries advance by a fixed
// increment so the retry sequence is reproducible.
func NextRetryClock(nowMs int64) int64 { return nowMs + 1 }

// DeviceResult is the raw, unvalidated device submission parsed by the HTTP
// layer. A successful, validated result becomes evidence; a failed, timed-out,
// malformed, or contradictory result only becomes a device-call record.
type DeviceResult struct {
	TrialID      string `json:"trial_id"`
	Round        int    `json:"round"`
	LogicalMs    int64  `json:"logical_ms"`
	Kind         string `json:"kind"` // "sample" or "valve"
	Channel      string `json:"channel"`
	PressurePa   int64  `json:"pressure_pa"`
	TempMC       int64  `json:"temp_mc"`
	FlowULPerS   int64  `json:"flow_ul_per_s"`
	ValveID      string `json:"valve_id"`
	CommandedPos int    `json:"commanded_pos"`
	ValvePos     int    `json:"valve_pos"`
	CommandedMs  int64  `json:"commanded_ms"`
	Error        string `json:"error"`
}
