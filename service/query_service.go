package service

import (
	"context"

	"abyssal-pressure-housing-qualification/evidence"
	"abyssal-pressure-housing-qualification/qualification"
	"abyssal-pressure-housing-qualification/trial"
)

// ListWindows returns the committed evidence windows for a trial round.
func (s *Service) ListWindows(ctx context.Context, trialID string, round int) ([]evidence.EvidenceWindow, error) {
	return s.store.ListWindows(ctx, trialID, round)
}

// ListValveReceipts returns the valve receipts for a trial round.
func (s *Service) ListValveReceipts(ctx context.Context, trialID string, round int) ([]evidence.ValveReceipt, error) {
	return s.store.ListValveReceipts(ctx, trialID, round)
}

// ListLeases returns the leases for a trial round.
func (s *Service) ListLeases(ctx context.Context, trialID string, round int) ([]trial.Lease, error) {
	return s.store.ListLeases(ctx, trialID, round)
}

// ListBindings returns the component bindings for a trial round.
func (s *Service) ListBindings(ctx context.Context, trialID string, round int) ([]trial.Binding, error) {
	return s.store.ListBindings(ctx, trialID, round)
}

// ListReviews returns the reviews for a trial round.
func (s *Service) ListReviews(ctx context.Context, trialID string, round int) ([]qualification.Review, error) {
	return s.store.ListReviews(ctx, trialID, round)
}
