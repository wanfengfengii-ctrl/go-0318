package service

import (
	"context"
	"encoding/json"
	"errors"

	"abyssal-pressure-housing-qualification/qualification"
	"abyssal-pressure-housing-qualification/store"
	"abyssal-pressure-housing-qualification/trial"
)

// SubmitReviewRequest submits an independent review by a qualified operator.
type SubmitReviewRequest struct {
	TrialID       string `json:"trial_id"`
	Operator      string `json:"operator"`
	Qualification string `json:"qualification"`
	QualExpiresAt int64  `json:"qual_expires_at"`
	OpNo          string `json:"op_no"`
}

// SubmitReview records a review stamped with the current logical instant. One
// review per operator per round is retained.
func (s *Service) SubmitReview(ctx context.Context, req SubmitReviewRequest) error {
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
	r := qualification.Review{
		TrialID: req.TrialID, Round: t.Round, Operator: req.Operator,
		Qualification: req.Qualification, ValidAtMs: s.now(),
		QualExpiresAt: req.QualExpiresAt,
	}
	if err := s.store.SaveReview(ctx, r); err != nil {
		return err
	}
	if err := s.appendEvent(ctx, t, trial.EventReviewSubmitted, r); err != nil {
		return err
	}
	return s.saveIdem(ctx, op, 200, r)
}

// FinalizeRequest competes for the admission terminal state.
type FinalizeRequest struct {
	TrialID string `json:"trial_id"`
	OpNo    string `json:"op_no"`
}

// Finalize evaluates every admission precondition and, when satisfied, commits
// the admitted terminal state and issues the unique credential atomically.
func (s *Service) Finalize(ctx context.Context, req FinalizeRequest) (*qualification.Credential, error) {
	op, err := operationOf(req)
	if err != nil {
		return nil, err
	}
	if done, body, err := s.checkIdem(ctx, op); err != nil {
		return nil, err
	} else if done {
		var c qualification.Credential
		if err := json.Unmarshal(body, &c); err != nil {
			return nil, err
		}
		return &c, nil
	}

	t, err := s.store.GetTrial(ctx, req.TrialID)
	if err != nil {
		return nil, err
	}
	snap, err := s.store.GetConfiguration(ctx, t.ConfigDigest)
	if err != nil {
		return nil, err
	}

	if err := s.checkAdmission(ctx, t); err != nil {
		return nil, err
	}

	root, err := s.computeEvidenceRoot(ctx, req.TrialID, t.Round)
	if err != nil {
		return nil, err
	}
	cred := qualification.Credential{
		TrialID:    req.TrialID,
		Digest:     qualification.DeriveCredential(snap.Digest, req.TrialID, root),
		IssuedAtMs: s.now(),
	}

	updated, err := s.commitTerminal(ctx, req.TrialID, t.Version, trial.TerminalAdmitted, &cred)
	if err != nil {
		return nil, err
	}
	if err := s.appendEvent(ctx, updated, trial.EventTerminal, map[string]any{"terminal": string(trial.TerminalAdmitted)}); err != nil {
		return nil, err
	}
	if err := s.appendEvent(ctx, updated, trial.EventCredentialIssued, cred); err != nil {
		return nil, err
	}
	if err := s.saveIdem(ctx, op, 200, cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

// TerminateRequest competes for the terminated terminal state.
type TerminateRequest struct {
	TrialID string `json:"trial_id"`
	OpNo    string `json:"op_no"`
}

// Terminate commits the terminated terminal state without issuing a credential.
func (s *Service) Terminate(ctx context.Context, req TerminateRequest) (*trial.Trial, error) {
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
	updated, err := s.commitTerminal(ctx, req.TrialID, t.Version, trial.TerminalTerminated, nil)
	if err != nil {
		return nil, err
	}
	if err := s.appendEvent(ctx, updated, trial.EventTerminal, map[string]any{"terminal": string(trial.TerminalTerminated)}); err != nil {
		return nil, err
	}
	if err := s.saveIdem(ctx, op, 200, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

// GetCredential returns the issued credential for a trial.
func (s *Service) GetCredential(ctx context.Context, trialID string) (*qualification.Credential, error) {
	return s.store.GetCredential(ctx, trialID)
}

// checkAdmission validates every precondition required for admission.
func (s *Service) checkAdmission(ctx context.Context, t *trial.Trial) error {
	windows, err := s.store.ListWindows(ctx, t.ID, t.Round)
	if err != nil {
		return err
	}
	samples, err := s.store.ListSamples(ctx, t.ID, t.Round)
	if err != nil {
		return err
	}
	reviews, err := s.store.ListReviews(ctx, t.ID, t.Round)
	if err != nil {
		return err
	}
	retest, err := s.store.GetRetestSet(ctx, t.ID, t.Round)
	retestCleared := true
	if err == nil {
		retestCleared = len(retest.Members) == 0
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}

	vented := len(samples) > 0 && samples[len(samples)-1].PressurePa == 0
	checksPassed := trial.StageIndex(t.Stage) >= trial.StageIndex(trial.StageReview)

	pre := qualification.AdmissionPreconditions{
		ConfigConsistent: true,
		AllStepsComplete: t.AllStepsComplete(),
		EvidenceNoGaps:   len(windows) == t.StepsTotal,
		VentedToZero:     vented,
		ChecksPassed:     checksPassed,
		RetestCleared:    retestCleared,
		Reviews:          reviews,
	}
	return pre.Validate()
}

// computeEvidenceRoot folds the committed evidence windows into a single
// deterministic root digest.
func (s *Service) computeEvidenceRoot(ctx context.Context, trialID string, round int) (string, error) {
	windows, err := s.store.ListWindows(ctx, trialID, round)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(windows))
	for _, w := range windows {
		raw, err := json.Marshal(w)
		if err != nil {
			return "", err
		}
		parts = append(parts, string(raw))
	}
	return qualification.EvidenceRoot(parts...), nil
}

// commitTerminal retries the terminal commit on version conflicts.
func (s *Service) commitTerminal(ctx context.Context, trialID string, version int64, ts trial.TerminalState, cred *qualification.Credential) (*trial.Trial, error) {
	for i := 0; i < 5; i++ {
		updated, err := s.store.CommitTerminal(ctx, trialID, version, ts, cred)
		if errors.Is(err, store.ErrVersionConflict) {
			cur, gerr := s.store.GetTrial(ctx, trialID)
			if gerr != nil {
				return nil, gerr
			}
			version = cur.Version
			continue
		}
		return updated, err
	}
	return nil, store.ErrStoreBusy
}
