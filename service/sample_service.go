package service

import (
	"context"

	"abyssal-pressure-housing-qualification/evidence"
	"abyssal-pressure-housing-qualification/trial"
)

// SubmitSampleRequest carries a raw integer sample for the current round.
type SubmitSampleRequest struct {
	TrialID    string `json:"trial_id"`
	Round      int    `json:"round"`
	LogicalMs  int64  `json:"logical_ms"`
	PressurePa int64  `json:"pressure_pa"`
	TempMC     int64  `json:"temp_mc"`
	FlowULPerS int64  `json:"flow_ul_per_s"`
	ValvePos   int    `json:"valve_pos"`
	OpNo       string `json:"op_no"`
}

// SubmitSample appends a raw sample after validating the logical-clock order,
// the current round, and that at least one active lease remains unexpired.
func (s *Service) SubmitSample(ctx context.Context, req SubmitSampleRequest) error {
	op, err := operationOf(req)
	if err != nil {
		return err
	}
	if done, _, err := s.checkIdem(ctx, op); err != nil {
		return err
	} else if done {
		return nil
	}

	t, err := s.store.GetTrial(ctx, req.TrialID)
	if err != nil {
		return err
	}
	if t.Terminal != trial.TerminalNone {
		return trial.ErrAlreadyTerminal
	}
	if req.Round != 0 && req.Round != t.Round {
		return evidence.ErrRoundStale
	}

	// Lease validity: at least one active lease must still cover this instant.
	leases, err := s.store.ListLeases(ctx, req.TrialID, t.Round)
	if err != nil {
		return err
	}
	valid := false
	for _, l := range leases {
		if l.Active {
			if err := evidence.ValidateLease(t.Round, l.Round, req.TrialID, l.Holder, l.ExpiresAt, req.LogicalMs); err == nil {
				valid = true
				break
			}
		}
	}
	if !valid {
		return evidence.ErrLeaseExpired
	}

	// Strictly increasing logical clock within the round.
	samples, err := s.store.ListSamples(ctx, req.TrialID, t.Round)
	if err != nil {
		return err
	}
	if len(samples) > 0 {
		last := samples[len(samples)-1]
		if err := evidence.ValidateSampleOrder(last.LogicalMs, req.LogicalMs); err != nil {
			return err
		}
	}

	seq, err := s.store.NextSeq(ctx, req.TrialID)
	if err != nil {
		return err
	}
	sm := evidence.Sample{
		TrialID: req.TrialID, Round: t.Round, Seq: seq,
		LogicalMs: req.LogicalMs, PressurePa: req.PressurePa,
		TempMC: req.TempMC, FlowULPerS: req.FlowULPerS, ValvePos: req.ValvePos,
	}
	if err := s.store.AppendSample(ctx, sm); err != nil {
		return err
	}
	if err := s.appendEvent(ctx, t, trial.EventSampleAppended, sm); err != nil {
		return err
	}
	return s.saveIdem(ctx, op, 200, sm)
}

// SubmitDeviceResultRequest carries a device acknowledgement or failure.
type SubmitDeviceResultRequest struct {
	TrialID      string `json:"trial_id"`
	Round        int    `json:"round"`
	LogicalMs    int64  `json:"logical_ms"`
	Kind         string `json:"kind"`
	Channel      string `json:"channel"`
	PressurePa   int64  `json:"pressure_pa"`
	TempMC       int64  `json:"temp_mc"`
	FlowULPerS   int64  `json:"flow_ul_per_s"`
	ValveID      string `json:"valve_id"`
	CommandedPos int    `json:"commanded_pos"`
	ValvePos     int    `json:"valve_pos"`
	CommandedMs  int64  `json:"commanded_ms"`
	Error        string `json:"error"`
	OpNo         string `json:"op_no"`
}

// SubmitDeviceResult routes a device acknowledgement. Failures (timeouts,
// calibration expiry, malformed kinds, or contradictory valve positions) become
// deterministic device-call records with an increasing retry number; only a
// validated valve acknowledgement becomes evidence. A device failure can never
// produce a qualified pressure or valve evidence record.
func (s *Service) SubmitDeviceResult(ctx context.Context, req SubmitDeviceResultRequest) error {
	op, err := operationOf(req)
	if err != nil {
		return err
	}
	if done, _, err := s.checkIdem(ctx, op); err != nil {
		return err
	} else if done {
		return nil
	}

	t, err := s.store.GetTrial(ctx, req.TrialID)
	if err != nil {
		return err
	}
	if t.Terminal != trial.TerminalNone {
		return trial.ErrAlreadyTerminal
	}
	if req.Round != 0 && req.Round != t.Round {
		return evidence.ErrRoundStale
	}

	// Classify the result. Any failure path produces a device call, not evidence.
	// The call record is itself part of the evidence chain, so the completed
	// failure is recorded for idempotency: an identical retry with the same
	// operation number must replay as a no-op instead of appending a second
	// retry record.
	if reason, failed := s.classifyDevice(req); failed {
		if err := s.recordDeviceCall(ctx, t, req.LogicalMs, reason); err != nil {
			return err
		}
		return s.saveIdem(ctx, op, 200, map[string]bool{"ok": true})
	}
	if req.Kind == "sample" {
		if stale, err := s.calibrationStale(ctx, t, req.Channel, req.LogicalMs); err != nil {
			return err
		} else if stale {
			if err := s.recordDeviceCall(ctx, t, req.LogicalMs, "calibration_stale"); err != nil {
				return err
			}
			return s.saveIdem(ctx, op, 200, map[string]bool{"ok": true})
		}
	}

	switch req.Kind {
	case "valve":
		delay, err := evidence.ValveDelay(req.CommandedMs, req.LogicalMs)
		if err != nil {
			return err
		}
		seq, err := s.store.NextSeq(ctx, req.TrialID)
		if err != nil {
			return err
		}
		v := evidence.ValveReceipt{
			TrialID: req.TrialID, Round: t.Round, Seq: seq,
			LogicalMs: req.LogicalMs, ValveID: req.ValveID,
			Position: req.ValvePos, DelayMs: delay,
		}
		if err := s.store.AppendValveReceipt(ctx, v); err != nil {
			return err
		}
		if err := s.appendEvent(ctx, t, trial.EventValveReceipt, v); err != nil {
			return err
		}
		return s.saveIdem(ctx, op, 200, v)
	default:
		if err := s.recordDeviceCall(ctx, t, req.LogicalMs, "format_error"); err != nil {
			return err
		}
		return s.saveIdem(ctx, op, 200, map[string]bool{"ok": true})
	}
}

// calibrationStale reports whether the sensor channel's frozen calibration has
// expired at the given logical instant.
func (s *Service) calibrationStale(ctx context.Context, t *trial.Trial, channel string, nowMs int64) (bool, error) {
	snap, err := s.store.GetConfiguration(ctx, t.ConfigDigest)
	if err != nil {
		return false, err
	}
	for _, c := range snap.Calibrations {
		if c.Channel == channel {
			return evidence.ValidateCalibration(c.ExpiresAtMs, nowMs) != nil, nil
		}
	}
	return false, nil
}

// classifyDevice returns a failure reason when the device result is not a valid,
// qualified acknowledgement.
func (s *Service) classifyDevice(req SubmitDeviceResultRequest) (string, bool) {
	if req.Error != "" {
		return req.Error, true
	}
	if req.Kind == "valve" {
		if req.CommandedPos != req.ValvePos {
			return "valve_mismatch", true
		}
		return "", false
	}
	if req.Kind != "sample" {
		return "format_error", true
	}
	return "", false
}

// recordDeviceCall appends a deterministic device retry record with the next
// retry number and next logical instant.
func (s *Service) recordDeviceCall(ctx context.Context, t *trial.Trial, logicalMs int64, reason string) error {
	calls, err := s.store.ListDeviceCalls(ctx, t.ID, t.Round)
	if err != nil {
		return err
	}
	retryNo := len(calls) + 1
	seq, err := s.store.NextSeq(ctx, t.ID)
	if err != nil {
		return err
	}
	call := evidence.DeviceCall{
		TrialID: t.ID, Round: t.Round, Seq: seq,
		LogicalMs: logicalMs, RetryNo: retryNo,
		NextLogicalMs: evidence.NextRetryClock(logicalMs),
		Kind:          "device", Reason: reason,
	}
	if err := s.store.AppendDeviceCall(ctx, call); err != nil {
		return err
	}
	return s.appendEvent(ctx, t, trial.EventDeviceCall, call)
}

// ListDeviceCalls returns the deterministic retry records for a trial round.
func (s *Service) ListDeviceCalls(ctx context.Context, trialID string, round int) ([]evidence.DeviceCall, error) {
	return s.store.ListDeviceCalls(ctx, trialID, round)
}

// ListSamples returns the raw samples for a trial round.
func (s *Service) ListSamples(ctx context.Context, trialID string, round int) ([]evidence.Sample, error) {
	return s.store.ListSamples(ctx, trialID, round)
}
