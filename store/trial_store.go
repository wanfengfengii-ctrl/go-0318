package store

import (
	"context"
	"database/sql"
	"fmt"

	"abyssal-pressure-housing-qualification/trial"
)

// CreateTrial inserts a new trial aggregate row.
func (s *SQLite) CreateTrial(ctx context.Context, t *trial.Trial) error {
	return withRetry(func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO trials (id, config_digest, stage, step_index, steps_total, round, terminal, version, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.ConfigDigest, string(t.Stage), t.StepIndex, t.StepsTotal, t.Round, string(t.Terminal), t.Version, nowMs(),
		)
		if err != nil {
			return fmt.Errorf("create trial: %w", err)
		}
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO trial_sequences (trial_id, seq) VALUES (?, 0)`, t.ID,
		)
		if err != nil {
			return fmt.Errorf("init trial sequence: %w", err)
		}
		return nil
	})
}

// GetTrial loads a trial aggregate row.
func (s *SQLite) GetTrial(ctx context.Context, id string) (*trial.Trial, error) {
	var t trial.Trial
	var stage, terminal string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, config_digest, stage, step_index, steps_total, round, terminal, version
		 FROM trials WHERE id = ?`, id,
	).Scan(&t.ID, &t.ConfigDigest, &stage, &t.StepIndex, &t.StepsTotal, &t.Round, &terminal, &t.Version)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get trial: %w", err)
	}
	t.Stage = trial.Stage(stage)
	t.Terminal = trial.TerminalState(terminal)
	return &t, nil
}

// UpdateTrial loads a trial, applies mutate inside a transaction, and commits
// the new state only if the optimistic version still matches. A concurrent
// writer causes ErrVersionConflict so the caller may retry deterministically.
func (s *SQLite) UpdateTrial(ctx context.Context, id string, expectedVersion int64, mutate func(*trial.Trial) error) (*trial.Trial, error) {
	var out *trial.Trial
	err := withRetry(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin: %w", err)
		}
		defer tx.Rollback()

		var t trial.Trial
		var stage, terminal string
		err = tx.QueryRowContext(ctx,
			`SELECT id, config_digest, stage, step_index, steps_total, round, terminal, version
			 FROM trials WHERE id = ?`, id,
		).Scan(&t.ID, &t.ConfigDigest, &stage, &t.StepIndex, &t.StepsTotal, &t.Round, &terminal, &t.Version)
		if err != nil {
			if err == sql.ErrNoRows {
				return ErrNotFound
			}
			return fmt.Errorf("get trial: %w", err)
		}
		t.Stage = trial.Stage(stage)
		t.Terminal = trial.TerminalState(terminal)
		if t.Version != expectedVersion {
			return ErrVersionConflict
		}
		if err := mutate(&t); err != nil {
			return err
		}
		t.Version++
		res, err := tx.ExecContext(ctx,
			`UPDATE trials SET stage = ?, step_index = ?, round = ?, terminal = ?, version = ?
			 WHERE id = ? AND version = ?`,
			string(t.Stage), t.StepIndex, t.Round, string(t.Terminal), t.Version, id, expectedVersion,
		)
		if err != nil {
			return fmt.Errorf("update trial: %w", err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrVersionConflict
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
		out = &t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AppendEvent appends a committed trial event to the append-only stream.
func (s *SQLite) AppendEvent(ctx context.Context, trialID string, e trial.Event) error {
	return withRetry(func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO trial_events (trial_id, seq, round, kind, payload) VALUES (?, ?, ?, ?, ?)`,
			trialID, e.Seq, e.Round, string(e.Kind), string(e.Payload),
		)
		if err != nil {
			return fmt.Errorf("append event: %w", err)
		}
		return nil
	})
}

// ListEvents returns the committed event stream for a trial in sequence order.
func (s *SQLite) ListEvents(ctx context.Context, trialID string) ([]trial.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT seq, round, kind, payload FROM trial_events WHERE trial_id = ? ORDER BY seq ASC`, trialID,
	)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()
	var out []trial.Event
	for rows.Next() {
		var e trial.Event
		var kind, payload string
		if err := rows.Scan(&e.Seq, &e.Round, &kind, &payload); err != nil {
			return nil, err
		}
		e.Kind = trial.EventKind(kind)
		e.Payload = []byte(payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

// NextSeq atomically allocates the next sequence number for a trial's event and
// evidence stream.
func (s *SQLite) NextSeq(ctx context.Context, trialID string) (int64, error) {
	var seq int64
	err := withRetry(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO trial_sequences (trial_id, seq) VALUES (?, 0) ON CONFLICT(trial_id) DO NOTHING`, trialID,
		); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT seq FROM trial_sequences WHERE trial_id = ?`, trialID,
		).Scan(&seq); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE trial_sequences SET seq = seq + 1 WHERE trial_id = ?`, trialID,
		); err != nil {
			return err
		}
		return tx.Commit()
	})
	if err != nil {
		return 0, fmt.Errorf("next seq: %w", err)
	}
	return seq, nil
}
