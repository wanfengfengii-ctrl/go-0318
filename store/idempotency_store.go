package store

import (
	"context"
	"database/sql"
	"fmt"

	"abyssal-pressure-housing-qualification/trial"
)

// GetIdempotency returns a stored write result by operation number, or
// ErrNotFound when the operation has not been seen before.
func (s *SQLite) GetIdempotency(ctx context.Context, opNo string) (*trial.IdempotencyRecord, error) {
	var rec trial.IdempotencyRecord
	var response string
	err := s.db.QueryRowContext(ctx,
		`SELECT op_no, digest, status_code, response FROM idempotency_records WHERE op_no = ?`, opNo,
	).Scan(&rec.OpNo, &rec.Digest, &rec.StatusCode, &response)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get idempotency: %w", err)
	}
	rec.Response = []byte(response)
	return &rec, nil
}

// SaveIdempotency records the result of a completed write, keyed by operation
// number, so an identical retry returns the original result.
func (s *SQLite) SaveIdempotency(ctx context.Context, rec trial.IdempotencyRecord) error {
	return withRetry(func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO idempotency_records (op_no, digest, status_code, response) VALUES (?, ?, ?, ?)`,
			rec.OpNo, rec.Digest, rec.StatusCode, string(rec.Response),
		)
		if err != nil {
			return fmt.Errorf("save idempotency: %w", err)
		}
		return nil
	})
}
