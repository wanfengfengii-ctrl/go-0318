package service

import (
	"context"
	"encoding/json"
	"errors"

	"abyssal-pressure-housing-qualification/store"
	"abyssal-pressure-housing-qualification/trial"
)

// CreateTrialRequest carries the frozen configuration and an optional operation
// number for idempotent creation.
type CreateTrialRequest struct {
	ID           string `json:"id"`
	ConfigDigest string `json:"config_digest"`
	OpNo         string `json:"op_no"`
}

// CreateTrial creates a new pressure-trial aggregate in the precheck stage.
func (s *Service) CreateTrial(ctx context.Context, req CreateTrialRequest) (*trial.Trial, error) {
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

	snap, err := s.store.GetConfiguration(ctx, req.ConfigDigest)
	if err != nil {
		return nil, err
	}
	id := req.ID
	if id == "" {
		id = newID()
	}
	t := trial.NewTrial(id, snap.Digest, len(snap.Steps))

	if err := s.store.CreateTrial(ctx, t); err != nil {
		return nil, err
	}
	seq, err := s.store.NextSeq(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	ev, err := trial.NewEvent(seq, t.Round, trial.EventTrialCreated, map[string]any{
		"id": t.ID, "config_digest": t.ConfigDigest, "steps_total": t.StepsTotal,
	})
	if err != nil {
		return nil, err
	}
	if err := s.store.AppendEvent(ctx, t.ID, ev); err != nil {
		return nil, err
	}
	if err := s.saveIdem(ctx, op, 200, t); err != nil {
		return nil, err
	}
	return t, nil
}

// GetTrial returns the current materialised state of a trial.
func (s *Service) GetTrial(ctx context.Context, id string) (*trial.Trial, error) {
	return s.store.GetTrial(ctx, id)
}

// StageRequest advances the trial to a later stage.
type StageRequest struct {
	TrialID string `json:"trial_id"`
	Stage   string `json:"stage"`
	OpNo    string `json:"op_no"`
}

// AdvanceStage moves the trial stage forward, preserving the continuous-prefix
// invariant. Skips and backward moves are rejected.
func (s *Service) AdvanceStage(ctx context.Context, req StageRequest) (*trial.Trial, error) {
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

	next := trial.Stage(req.Stage)
	if !trial.ValidStage(next) {
		return nil, &trial.StageTransitionError{To: next}
	}

	updated, err := s.updateWithEvent(ctx, req.TrialID, func(t *trial.Trial) error {
		return t.AdvanceStage(next)
	}, func(t *trial.Trial) (trial.EventKind, any) {
		return trial.EventStageAdvanced, map[string]any{"stage": string(next)}
	})
	if err != nil {
		return nil, err
	}
	if err := s.saveIdem(ctx, op, 200, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

// RestartRoundRequest starts a fresh round after reassembly.
type RestartRoundRequest struct {
	TrialID string `json:"trial_id"`
	OpNo    string `json:"op_no"`
}

// RestartRound starts a new round: the round increments, the stage resets to
// precheck, and the step ladder restarts. Old rounds stay read-only. The old
// round's active bindings and leases are released so the same hardware (pump
// groups, sensors) can be re-bound and re-leased on the new round; the released
// rows stay in place as read-only history rather than being deleted.
func (s *Service) RestartRound(ctx context.Context, req RestartRoundRequest) (*trial.Trial, error) {
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
	// Capture the round being closed before NewRound increments the counter;
	// this is the round whose device occupancy must be released.
	var priorRound int
	updated, err := s.updateWithEvent(ctx, req.TrialID, func(t *trial.Trial) error {
		priorRound = t.Round
		return t.NewRound()
	}, func(t *trial.Trial) (trial.EventKind, any) {
		return trial.EventRoundStarted, map[string]any{"round": t.Round}
	})
	if err != nil {
		return nil, err
	}
	// Release the closed round's bindings and leases so the operator can
	// re-open the trial with the same hardware on the new round without
	// colliding with the still-active old-round occupancy.
	if err := s.store.ReleaseRound(ctx, req.TrialID, priorRound); err != nil {
		return nil, err
	}
	if err := s.saveIdem(ctx, op, 200, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

// updateWithEvent runs mutate inside an optimistic-concurrency update, retrying
// on version conflicts, then appends the event derived from the updated state.
// The event is appended after the state commit so it never opens a nested
// transaction against the single shared connection.
func (s *Service) updateWithEvent(ctx context.Context, trialID string, mutate func(*trial.Trial) error, event func(*trial.Trial) (trial.EventKind, any)) (*trial.Trial, error) {
	var updated *trial.Trial
	for i := 0; i < 5; i++ {
		cur, err := s.store.GetTrial(ctx, trialID)
		if err != nil {
			return nil, err
		}
		updated, err = s.store.UpdateTrial(ctx, trialID, cur.Version, mutate)
		if errors.Is(err, store.ErrVersionConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		break
	}
	if updated == nil {
		return nil, store.ErrStoreBusy
	}
	kind, payload := event(updated)
	if err := s.appendEvent(ctx, updated, kind, payload); err != nil {
		return nil, err
	}
	return updated, nil
}

// appendEvent allocates the next sequence number and appends an event for the
// trial's current round.
func (s *Service) appendEvent(ctx context.Context, t *trial.Trial, kind trial.EventKind, payload any) error {
	seq, err := s.store.NextSeq(ctx, t.ID)
	if err != nil {
		return err
	}
	ev, err := trial.NewEvent(seq, t.Round, kind, payload)
	if err != nil {
		return err
	}
	return s.store.AppendEvent(ctx, t.ID, ev)
}
