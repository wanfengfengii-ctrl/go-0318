package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"abyssal-pressure-housing-qualification/qualification"
)

// SaveRetestSet stores (or replaces) the retest scope for a trial round,
// keyed by anomaly kind and carrying the canonically ordered members.
func (s *SQLite) SaveRetestSet(ctx context.Context, rs qualification.RetestSet) error {
	raw, err := json.Marshal(rs.Members)
	if err != nil {
		return fmt.Errorf("marshal retest set: %w", err)
	}
	return withRetry(func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT OR REPLACE INTO retest_sets (trial_id, round, anomaly_kind, members_json, cleared)
			 VALUES (?, ?, ?, ?, 0)`,
			rs.TrialID, rs.Round, "anomaly", string(raw),
		)
		if err != nil {
			return fmt.Errorf("save retest set: %w", err)
		}
		return nil
	})
}

// GetRetestSet returns the retest scope for a trial round, or ErrNotFound.
func (s *SQLite) GetRetestSet(ctx context.Context, trialID string, round int) (*qualification.RetestSet, error) {
	var raw string
	var cleared int
	err := s.db.QueryRowContext(ctx,
		`SELECT members_json, cleared FROM retest_sets WHERE trial_id = ? AND round = ?`, trialID, round,
	).Scan(&raw, &cleared)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get retest set: %w", err)
	}
	rs := &qualification.RetestSet{TrialID: trialID, Round: round}
	if err := json.Unmarshal([]byte(raw), &rs.Members); err != nil {
		return nil, err
	}
	if cleared != 0 {
		rs.Members = nil
	}
	return rs, nil
}

// ClearRetestSet marks the retest scope for a trial round as cleared, leaving
// the historical record intact for audit.
func (s *SQLite) ClearRetestSet(ctx context.Context, trialID string, round int) error {
	return withRetry(func() error {
		_, err := s.db.ExecContext(ctx,
			`UPDATE retest_sets SET cleared = 1 WHERE trial_id = ? AND round = ?`, trialID, round,
		)
		if err != nil {
			return fmt.Errorf("clear retest set: %w", err)
		}
		return nil
	})
}

// SaveReview stores an independent review. The primary key (trial, round,
// operator) enforces one review per operator per round.
func (s *SQLite) SaveReview(ctx context.Context, r qualification.Review) error {
	return withRetry(func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT OR REPLACE INTO reviews (trial_id, round, operator, qualification, valid_at_ms, qual_expires_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			r.TrialID, r.Round, r.Operator, r.Qualification, r.ValidAtMs, r.QualExpiresAt,
		)
		if err != nil {
			return fmt.Errorf("save review: %w", err)
		}
		return nil
	})
}

// ListReviews returns every review for a trial round, ordered by operator.
func (s *SQLite) ListReviews(ctx context.Context, trialID string, round int) ([]qualification.Review, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT round, operator, qualification, valid_at_ms, qual_expires_at
		 FROM reviews WHERE trial_id = ? AND round = ? ORDER BY operator ASC`, trialID, round,
	)
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	defer rows.Close()
	var out []qualification.Review
	for rows.Next() {
		var r qualification.Review
		r.TrialID = trialID
		if err := rows.Scan(&r.Round, &r.Operator, &r.Qualification, &r.ValidAtMs, &r.QualExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SaveCredential stores the unique admission credential for a trial.
func (s *SQLite) SaveCredential(ctx context.Context, c qualification.Credential) error {
	return withRetry(func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO admission_credentials (trial_id, digest, issued_at_ms) VALUES (?, ?, ?)`,
			c.TrialID, c.Digest, c.IssuedAtMs,
		)
		if err != nil {
			return fmt.Errorf("save credential: %w", err)
		}
		return nil
	})
}

// GetCredential returns the admission credential for a trial, or ErrNotFound.
func (s *SQLite) GetCredential(ctx context.Context, trialID string) (*qualification.Credential, error) {
	var c qualification.Credential
	c.TrialID = trialID
	err := s.db.QueryRowContext(ctx,
		`SELECT digest, issued_at_ms FROM admission_credentials WHERE trial_id = ?`, trialID,
	).Scan(&c.Digest, &c.IssuedAtMs)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get credential: %w", err)
	}
	return &c, nil
}
