package store

import (
	"context"
	"encoding/json"
	"fmt"

	"abyssal-pressure-housing-qualification/configuration"
)

// SaveConfiguration stores a frozen snapshot, keyed by its digest. The digest
// is stable, so storing an identical snapshot is idempotent.
func (s *SQLite) SaveConfiguration(ctx context.Context, snap *configuration.Snapshot) error {
	raw, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	return withRetry(func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO configuration_snapshots (digest, snapshot_json) VALUES (?, ?)`,
			snap.Digest, string(raw),
		)
		if err != nil {
			return fmt.Errorf("save configuration: %w", err)
		}
		return nil
	})
}

// GetConfiguration returns a stored snapshot by digest.
func (s *SQLite) GetConfiguration(ctx context.Context, digest string) (*configuration.Snapshot, error) {
	var raw string
	err := s.db.QueryRowContext(ctx,
		`SELECT snapshot_json FROM configuration_snapshots WHERE digest = ?`, digest,
	).Scan(&raw)
	if err != nil {
		return nil, fmt.Errorf("get configuration: %w", err)
	}
	var snap configuration.Snapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return &snap, nil
}
