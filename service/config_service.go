package service

import (
	"context"

	"abyssal-pressure-housing-qualification/configuration"
)

// FreezeConfiguration validates, canonicalises, and persists a configuration.
// The returned snapshot digest is stable across restarts, so freezing the same
// configuration again is idempotent.
func (s *Service) FreezeConfiguration(ctx context.Context, in configuration.Input) (*configuration.Snapshot, error) {
	snap, err := configuration.Freeze(in)
	if err != nil {
		return nil, err
	}
	if err := s.store.SaveConfiguration(ctx, snap); err != nil {
		return nil, err
	}
	return snap, nil
}

// GetConfiguration returns a frozen snapshot by digest.
func (s *Service) GetConfiguration(ctx context.Context, digest string) (*configuration.Snapshot, error) {
	return s.store.GetConfiguration(ctx, digest)
}
