package store

import (
	"context"
	"fmt"

	"abyssal-pressure-housing-qualification/trial"
)

// Startup atomically applies a set of component bindings and resource leases in
// one transaction. Any duplicate serial, duplicate position, or already-active
// lease violates a unique constraint and rolls the whole transaction back.
func (s *SQLite) Startup(ctx context.Context, trialID string, round int, bindings []trial.Binding, leases []trial.Lease) error {
	return withRetry(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin: %w", err)
		}
		defer tx.Rollback()

		for _, b := range bindings {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO bindings (trial_id, round, serial, type, position, active)
				 VALUES (?, ?, ?, ?, ?, 1)`,
				trialID, round, b.Serial, string(b.Type), b.Position,
			); err != nil {
				return fmt.Errorf("insert binding: %w", err)
			}
		}
		for _, l := range leases {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO leases (trial_id, round, resource_id, holder, token, expires_at, active)
				 VALUES (?, ?, ?, ?, ?, ?, 1)`,
				trialID, round, l.ResourceID, l.Holder, l.Token, l.ExpiresAt,
			); err != nil {
				return fmt.Errorf("insert lease: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
		return nil
	})
}

// RenewLease extends an active lease's expiry only when the holder and token
// still match, keeping the lease from being hijacked.
func (s *SQLite) RenewLease(ctx context.Context, trialID, resourceID, holder, token string, newExpiry int64) error {
	return withRetry(func() error {
		res, err := s.db.ExecContext(ctx,
			`UPDATE leases SET expires_at = ? WHERE trial_id = ? AND resource_id = ? AND holder = ? AND token = ? AND active = 1`,
			newExpiry, trialID, resourceID, holder, token,
		)
		if err != nil {
			return fmt.Errorf("renew lease: %w", err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrNotFound
		}
		return nil
	})
}

// ListLeases returns every lease for a trial round, ordered by resource id.
func (s *SQLite) ListLeases(ctx context.Context, trialID string, round int) ([]trial.Lease, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT trial_id, round, resource_id, holder, token, expires_at, active
		 FROM leases WHERE trial_id = ? AND round = ? ORDER BY resource_id ASC`, trialID, round,
	)
	if err != nil {
		return nil, fmt.Errorf("list leases: %w", err)
	}
	defer rows.Close()
	var out []trial.Lease
	for rows.Next() {
		var l trial.Lease
		if err := rows.Scan(&l.TrialID, &l.Round, &l.ResourceID, &l.Holder, &l.Token, &l.ExpiresAt, &l.Active); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ListBindings returns every binding for a trial round, ordered by serial.
func (s *SQLite) ListBindings(ctx context.Context, trialID string, round int) ([]trial.Binding, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT trial_id, round, serial, type, position FROM bindings WHERE trial_id = ? AND round = ? ORDER BY serial ASC`, trialID, round,
	)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	defer rows.Close()
	var out []trial.Binding
	for rows.Next() {
		var b trial.Binding
		var typ string
		if err := rows.Scan(&b.TrialID, &b.Round, &b.Serial, &typ, &b.Position); err != nil {
			return nil, err
		}
		b.Type = trial.ComponentType(typ)
		out = append(out, b)
	}
	return out, rows.Err()
}
