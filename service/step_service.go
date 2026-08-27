package service

import (
	"context"
	"encoding/json"

	"abyssal-pressure-housing-qualification/configuration"
	"abyssal-pressure-housing-qualification/evidence"
	"abyssal-pressure-housing-qualification/trial"
)

// CompleteStepRequest completes the current pressure step by evaluating the
// frozen half-open sampling window [StartMs, EndMs).
type CompleteStepRequest struct {
	TrialID   string `json:"trial_id"`
	StepIndex int    `json:"step_index"`
	StartMs   int64  `json:"start_ms"`
	EndMs     int64  `json:"end_ms"`
	OpNo      string `json:"op_no"`
}

// CompleteStep evaluates the evidence window for the current step against the
// frozen thresholds. When the window is qualified it is stored (one window per
// step) and the step ladder advances. Overpressure, rate, drop, or leak
// violations abort the transaction and leave no state behind.
func (s *Service) CompleteStep(ctx context.Context, req CompleteStepRequest) (*trial.Trial, error) {
	op, err := operationOf(req)
	if err != nil {
		return nil, err
	}
	if done, body, err := s.checkIdem(ctx, op); err != nil {
		return nil, err
	} else if done {
		var t trial.Trial
		if err := json.Unmarshal(body, &t); err != nil {
			return nil, err
		}
		return &t, nil
	}

	t, err := s.store.GetTrial(ctx, req.TrialID)
	if err != nil {
		return nil, err
	}
	if t.StepIndex != req.StepIndex {
		return nil, trial.ErrStepOutOfOrder
	}

	snap, err := s.store.GetConfiguration(ctx, t.ConfigDigest)
	if err != nil {
		return nil, err
	}
	step := snap.StepByIndex(req.StepIndex)
	if step == nil {
		return nil, trial.ErrStepOutOfOrder
	}

	// Build the half-open window from the round's samples.
	samples, err := s.store.ListSamples(ctx, req.TrialID, t.Round)
	if err != nil {
		return nil, err
	}
	var inWindow []evidence.Sample
	for _, sm := range samples {
		if evidence.InWindow(sm.LogicalMs, req.StartMs, req.EndMs) {
			inWindow = append(inWindow, sm)
		}
	}
	win := evidence.EvidenceWindow{
		TrialID: req.TrialID, Round: t.Round, StepIndex: req.StepIndex,
		StartMs: req.StartMs, EndMs: req.EndMs, Samples: inWindow,
	}
	params := evidence.StepParams{
		TargetPa: step.TargetPa, RampUpPaPerS: step.RampUpPaPerS,
		RampDownPaPerS: step.RampDownPaPerS, HoldMs: step.HoldMs,
		LeakLimitULPerS: step.LeakLimitULPerS, MaxDropPa: step.MaxDropPa,
		VolumeUL: s.inletVolumeUL(snap), MaxPressurePa: snap.MaxPressurePa,
	}
	if _, err := evidence.Evaluate(win, params, evidence.CompensationParams{
		RefTempMC: snap.Compensation.RefTempMC, TempCoeffPPM: snap.Compensation.TempCoeffPPM,
	}); err != nil {
		return nil, err
	}

	if err := s.store.SaveWindow(ctx, win); err != nil {
		return nil, err
	}

	updated, err := s.updateWithEvent(ctx, req.TrialID, func(tt *trial.Trial) error {
		return tt.CompleteStep(req.StepIndex)
	}, func(tt *trial.Trial) (trial.EventKind, any) {
		return trial.EventStepCompleted, map[string]any{"index": req.StepIndex}
	})
	if err != nil {
		return nil, err
	}
	if err := s.appendEvent(ctx, updated, trial.EventEvidenceWindow, win); err != nil {
		return nil, err
	}
	if err := s.saveIdem(ctx, op, 200, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

// inletVolumeUL returns the internal volume of the chamber holding the pressure
// inlet, used for leak-rate estimation.
func (s *Service) inletVolumeUL(snap *configuration.Snapshot) int64 {
	p := snap.PortByID(snap.InletPort())
	if p == nil {
		return 0
	}
	ch := snap.ChamberByID(p.Chamber)
	if ch == nil {
		return 0
	}
	return ch.VolumeUL
}
