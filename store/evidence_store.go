package store

import (
	"context"
	"encoding/json"
	"fmt"

	"abyssal-pressure-housing-qualification/evidence"
)

// SaveWindow stores a qualified evidence window for a trial round and step. The
// primary key (trial, round, step) enforces one window per step.
func (s *SQLite) SaveWindow(ctx context.Context, w evidence.EvidenceWindow) error {
	raw, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("marshal window: %w", err)
	}
	return withRetry(func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT OR REPLACE INTO evidence_windows (trial_id, round, step_index, start_ms, end_ms, window_json)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			w.TrialID, w.Round, w.StepIndex, w.StartMs, w.EndMs, string(raw),
		)
		if err != nil {
			return fmt.Errorf("save window: %w", err)
		}
		return nil
	})
}

// ListWindows returns every evidence window for a trial round, ordered by step.
func (s *SQLite) ListWindows(ctx context.Context, trialID string, round int) ([]evidence.EvidenceWindow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT window_json FROM evidence_windows WHERE trial_id = ? AND round = ? ORDER BY step_index ASC`, trialID, round,
	)
	if err != nil {
		return nil, fmt.Errorf("list windows: %w", err)
	}
	defer rows.Close()
	var out []evidence.EvidenceWindow
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var w evidence.EvidenceWindow
		if err := json.Unmarshal([]byte(raw), &w); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
