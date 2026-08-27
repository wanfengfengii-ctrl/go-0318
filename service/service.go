// Package service orchestrates the pressure-housing qualification use cases on
// top of the store and domain packages. It is the application layer: it owns
// the idempotency contract, the injected logical clock, and every business flow
// (configuration freeze, atomic startup, graded pressurisation, evidence
// judgement, anomaly retest, and admission issuance).
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"abyssal-pressure-housing-qualification/store"
	"abyssal-pressure-housing-qualification/trial"
)

// Service wires the store and an injected logical clock together.
type Service struct {
	store store.Store
	now   func() int64
}

// New constructs a Service with a wall-clock logical clock. Tests inject a
// deterministic clock via SetClock.
func New(st store.Store) *Service {
	return &Service{store: st, now: func() int64 { return time.Now().UnixMilli() }}
}

// SetClock replaces the logical clock source.
func (s *Service) SetClock(fn func() int64) { s.now = fn }

// Operation carries the caller-supplied operation number and the canonical
// request digest used for idempotent write deduplication.
type Operation = trial.Operation

// operationOf derives an Operation from a request value: the digest is the
// canonical, order-independent digest of the whole request (including OpNo).
func operationOf(v any) (Operation, error) {
	digest, err := trial.RequestDigest(v)
	if err != nil {
		return Operation{}, err
	}
	var m map[string]any
	raw, _ := json.Marshal(v)
	_ = json.Unmarshal(raw, &m)
	opNo, _ := m["op_no"].(string)
	return Operation{OpNo: opNo, Digest: digest}, nil
}

// checkIdem performs the idempotency lookup. It returns the stored response
// body (and done=true) when the operation was already completed with the same
// content, or ErrIdempotencyConflict when the operation number was reused with
// different content.
func (s *Service) checkIdem(ctx context.Context, op Operation) (done bool, body []byte, err error) {
	if op.OpNo == "" {
		return false, nil, nil
	}
	rec, err := s.store.GetIdempotency(ctx, op.OpNo)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	if rec.Digest != op.Digest {
		return false, nil, trial.ErrIdempotencyConflict
	}
	return true, rec.Response, nil
}

// saveIdem records a completed write so an identical retry returns the original
// result.
func (s *Service) saveIdem(ctx context.Context, op Operation, status int, body any) error {
	if op.OpNo == "" {
		return nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return s.store.SaveIdempotency(ctx, trial.IdempotencyRecord{
		OpNo: op.OpNo, Digest: op.Digest, StatusCode: status, Response: raw,
	})
}

// newID returns a random 128-bit identifier encoded as hex.
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}
