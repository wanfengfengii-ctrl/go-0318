package service

import (
	"context"
	"errors"
	"fmt"

	"abyssal-pressure-housing-qualification/store"
	"abyssal-pressure-housing-qualification/trial"
)

// StartupRequest atomically binds components to install positions and leases
// shared resources for a trial round.
type StartupRequest struct {
	TrialID  string          `json:"trial_id"`
	Bindings []trial.Binding `json:"bindings"`
	Leases   []trial.Lease   `json:"leases"`
	OpNo     string          `json:"op_no"`
}

// ErrStartupConflict is returned when a startup loses a uniqueness race: a
// serial, position, or resource is already bound or leased elsewhere.
var ErrStartupConflict = errors.New("startup conflict")

// Startup performs the atomic open-trial transaction. Every binding and lease
// succeeds or the whole operation rolls back; a competing startup for the same
// resource or position fails without leaving partial bindings behind.
func (s *Service) Startup(ctx context.Context, req StartupRequest) error {
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
	now := s.now()
	round := t.Round

	bindings := make([]trial.Binding, 0, len(req.Bindings))
	for _, b := range req.Bindings {
		b.TrialID = req.TrialID
		b.Round = round
		bindings = append(bindings, b)
	}
	leases := make([]trial.Lease, 0, len(req.Leases))
	for _, l := range req.Leases {
		l.TrialID = req.TrialID
		l.Round = round
		if l.Holder == "" {
			l.Holder = req.TrialID
		}
		if l.ExpiresAt == 0 {
			l.ExpiresAt = now + defaultLeaseDurationMs
		}
		if l.Token == "" {
			l.Token = newID()
		}
		leases = append(leases, l)
	}

	if err := s.store.Startup(ctx, req.TrialID, round, bindings, leases); err != nil {
		return fmt.Errorf("%w: %v", ErrStartupConflict, err)
	}

	for _, b := range bindings {
		if err := s.appendEvent(ctx, t, trial.EventBindingApplied, b); err != nil {
			return err
		}
	}
	for _, l := range leases {
		if err := s.appendEvent(ctx, t, trial.EventLeaseGranted, l); err != nil {
			return err
		}
	}

	return s.saveIdem(ctx, op, 200, map[string]any{"leases": leases, "bindings": bindings})
}

// RenewLeaseRequest extends an active lease's expiry.
type RenewLeaseRequest struct {
	TrialID      string `json:"trial_id"`
	ResourceID   string `json:"resource_id"`
	Holder       string `json:"holder"`
	Token        string `json:"token"`
	NewExpiresAt int64  `json:"new_expires_at"`
	OpNo         string `json:"op_no"`
}

// RenewLease extends a lease only when the holder and token match, preventing
// lease hijacking.
func (s *Service) RenewLease(ctx context.Context, req RenewLeaseRequest) error {
	op, err := operationOf(req)
	if err != nil {
		return err
	}
	if done, _, err := s.checkIdem(ctx, op); err != nil {
		return err
	} else if done {
		return nil
	}
	if err := s.store.RenewLease(ctx, req.TrialID, req.ResourceID, req.Holder, req.Token, req.NewExpiresAt); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return trial.ErrIdempotencyConflict // lease not found or mismatched token
		}
		return err
	}
	return s.saveIdem(ctx, op, 200, map[string]bool{"ok": true})
}

// defaultLeaseDurationMs is the default lease validity when a startup request
// omits an explicit expiry.
const defaultLeaseDurationMs = 60_000
