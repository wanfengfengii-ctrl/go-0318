package store

import (
	"context"
	"database/sql"
	"fmt"

	"abyssal-pressure-housing-qualification/qualification"
	"abyssal-pressure-housing-qualification/trial"
)

// CommitTerminal atomically commits a terminal state (and, for admission, the
// unique credential) in a single transaction. The terminal barrier permits
// exactly one outcome: if the trial is already terminal the call fails with
// ErrFinalStateConflict, and no second credential can ever be issued because
// the credential insert and the terminal update share one transaction.
func (s *SQLite) CommitTerminal(ctx context.Context, trialID string, expectedVersion int64, ts trial.TerminalState, cred *qualification.Credential) (*trial.Trial, error) {
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
			 FROM trials WHERE id = ?`, trialID,
		).Scan(&t.ID, &t.ConfigDigest, &stage, &t.StepIndex, &t.StepsTotal, &t.Round, &terminal, &t.Version)
		if err != nil {
			if err == sql.ErrNoRows {
				return ErrNotFound
			}
			return fmt.Errorf("get trial: %w", err)
		}
		t.Stage = trial.Stage(stage)
		t.Terminal = trial.TerminalState(terminal)
		if t.Terminal != trial.TerminalNone {
			return qualification.ErrFinalStateConflict
		}
		if t.Version != expectedVersion {
			return ErrVersionConflict
		}
		if cred != nil {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO admission_credentials (trial_id, digest, issued_at_ms) VALUES (?, ?, ?)`,
				cred.TrialID, cred.Digest, cred.IssuedAtMs,
			); err != nil {
				return qualification.ErrFinalStateConflict
			}
		}
		t.Terminal = ts
		t.Version++
		res, err := tx.ExecContext(ctx,
			`UPDATE trials SET terminal = ?, version = ? WHERE id = ? AND version = ?`,
			string(ts), t.Version, trialID, expectedVersion,
		)
		if err != nil {
			return fmt.Errorf("update terminal: %w", err)
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
