package service

import (
	"context"
	"encoding/json"
	"errors"

	"abyssal-pressure-housing-qualification/qualification"
	"abyssal-pressure-housing-qualification/store"
	"abyssal-pressure-housing-qualification/trial"
)

// ReportAnomalyRequest reports an isolation-triggering anomaly located on a
// port, chamber, or seal boundary.
type ReportAnomalyRequest struct {
	TrialID   string `json:"trial_id"`
	Kind      string `json:"kind"`
	PortID    string `json:"port_id"`
	ChamberID string `json:"chamber_id"`
	SealID    string `json:"seal_id"`
	OpNo      string `json:"op_no"`
}

// ReportAnomaly derives and persists the retest scope for an anomaly, merging
// with any previously reported scope for the same round. The result is
// canonically ordered and de-duplicated.
func (s *Service) ReportAnomaly(ctx context.Context, req ReportAnomalyRequest) (*qualification.RetestSet, error) {
	op, err := operationOf(req)
	if err != nil {
		return nil, err
	}
	if done, body, err := s.checkIdem(ctx, op); err != nil {
		return nil, err
	} else if done {
		var rs qualification.RetestSet
		if err := json.Unmarshal(body, &rs); err != nil {
			return nil, err
		}
		return &rs, nil
	}

	t, err := s.store.GetTrial(ctx, req.TrialID)
	if err != nil {
		return nil, err
	}
	snap, err := s.store.GetConfiguration(ctx, t.ConfigDigest)
	if err != nil {
		return nil, err
	}

	fresh, err := qualification.PropagateRetest(snap, qualification.Anomaly{
		Kind: qualification.AnomalyKind(req.Kind), PortID: req.PortID,
		ChamberID: req.ChamberID, SealID: req.SealID,
	})
	if err != nil {
		return nil, err
	}
	fresh.TrialID = req.TrialID
	fresh.Round = t.Round

	// Read-merge-save under optimistic concurrency. Concurrent anomaly
	// reports for the same round each re-read the latest scope and fold
	// their fresh members in; a save that loses the version race is retried
	// so both anomalies accumulate into one canonically ordered set rather
	// than the last writer clobbering the earlier scope.
	var merged qualification.RetestSet
	saved := false
	for i := 0; i < 5; i++ {
		merged = fresh
		if existing, err := s.store.GetRetestSet(ctx, req.TrialID, t.Round); err == nil {
			merged = qualification.MergeRetest(fresh, *existing)
			merged.TrialID = req.TrialID
			merged.Round = t.Round
			merged.Version = existing.Version
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		if err := s.store.SaveRetestSet(ctx, merged); err == nil {
			saved = true
			break
		} else if !errors.Is(err, store.ErrVersionConflict) {
			return nil, err
		}
		// Lost the race: another anomaly committed a newer version.
		// Re-read and re-merge, preserving that winner's members.
	}
	if !saved {
		return nil, store.ErrStoreBusy
	}

	if err := s.appendEvent(ctx, t, trial.EventRetestReported, merged); err != nil {
		return nil, err
	}
	if err := s.saveIdem(ctx, op, 200, merged); err != nil {
		return nil, err
	}
	return &merged, nil
}

// GetRetestSet returns the current retest scope for a trial round, or nil when
// none exists.
func (s *Service) GetRetestSet(ctx context.Context, trialID string, round int) (*qualification.RetestSet, error) {
	rs, err := s.store.GetRetestSet(ctx, trialID, round)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return rs, err
}

// ClearRetestSet marks the retest scope as cleared after reassembly.
func (s *Service) ClearRetestSet(ctx context.Context, trialID string, round int) error {
	return s.store.ClearRetestSet(ctx, trialID, round)
}
