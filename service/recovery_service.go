package service

import (
	"context"

	"abyssal-pressure-housing-qualification/trial"
)

// RecoverTrial reconstructs the materialised trial aggregate by replaying the
// committed, sequence-ordered event stream. It exercises the restart-recovery
// path: the same events always rebuild the same aggregate state, so a process
// restart cannot diverge from the persisted materialised state.
func (s *Service) RecoverTrial(ctx context.Context, trialID string) (*trial.Trial, error) {
	events, err := s.store.ListEvents(ctx, trialID)
	if err != nil {
		return nil, err
	}
	// Seed with the stored identity so replay applies every state transition.
	cur, err := s.store.GetTrial(ctx, trialID)
	if err != nil {
		return nil, err
	}
	base := trial.NewTrial(cur.ID, cur.ConfigDigest, cur.StepsTotal)
	if err := trial.Replay(base, events); err != nil {
		return nil, err
	}
	return base, nil
}
