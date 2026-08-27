package store

import (
	"context"
	"fmt"

	"abyssal-pressure-housing-qualification/evidence"
)

// AppendSample appends a raw integer sample to the trial round's evidence
// stream. The primary key (trial, round, seq) rejects duplicates.
func (s *SQLite) AppendSample(ctx context.Context, sm evidence.Sample) error {
	return withRetry(func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO samples (trial_id, round, seq, logical_ms, pressure_pa, temp_mc, flow_ul_per_s, valve_pos)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			sm.TrialID, sm.Round, sm.Seq, sm.LogicalMs, sm.PressurePa, sm.TempMC, sm.FlowULPerS, sm.ValvePos,
		)
		if err != nil {
			return fmt.Errorf("append sample: %w", err)
		}
		return nil
	})
}

// AppendDeviceCall appends a deterministic device retry record. Device failures
// never become evidence; they only add a call record with a fixed retry number.
func (s *SQLite) AppendDeviceCall(ctx context.Context, c evidence.DeviceCall) error {
	return withRetry(func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO device_calls (trial_id, round, seq, logical_ms, retry_no, next_logical_ms, kind, reason)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			c.TrialID, c.Round, c.Seq, c.LogicalMs, c.RetryNo, c.NextLogicalMs, c.Kind, c.Reason,
		)
		if err != nil {
			return fmt.Errorf("append device call: %w", err)
		}
		return nil
	})
}

// AppendValveReceipt appends a valve acknowledgement to the evidence stream.
func (s *SQLite) AppendValveReceipt(ctx context.Context, v evidence.ValveReceipt) error {
	return withRetry(func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO valve_receipts (trial_id, round, seq, logical_ms, valve_id, position, delay_ms)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			v.TrialID, v.Round, v.Seq, v.LogicalMs, v.ValveID, v.Position, v.DelayMs,
		)
		if err != nil {
			return fmt.Errorf("append valve receipt: %w", err)
		}
		return nil
	})
}

// ListSamples returns every sample for a trial round in sequence order.
func (s *SQLite) ListSamples(ctx context.Context, trialID string, round int) ([]evidence.Sample, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT trial_id, round, seq, logical_ms, pressure_pa, temp_mc, flow_ul_per_s, valve_pos
		 FROM samples WHERE trial_id = ? AND round = ? ORDER BY seq ASC`, trialID, round,
	)
	if err != nil {
		return nil, fmt.Errorf("list samples: %w", err)
	}
	defer rows.Close()
	var out []evidence.Sample
	for rows.Next() {
		var sm evidence.Sample
		if err := rows.Scan(&sm.TrialID, &sm.Round, &sm.Seq, &sm.LogicalMs, &sm.PressurePa, &sm.TempMC, &sm.FlowULPerS, &sm.ValvePos); err != nil {
			return nil, err
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

// ListDeviceCalls returns every device-call record for a trial round in
// sequence order.
func (s *SQLite) ListDeviceCalls(ctx context.Context, trialID string, round int) ([]evidence.DeviceCall, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT trial_id, round, seq, logical_ms, retry_no, next_logical_ms, kind, reason
		 FROM device_calls WHERE trial_id = ? AND round = ? ORDER BY seq ASC`, trialID, round,
	)
	if err != nil {
		return nil, fmt.Errorf("list device calls: %w", err)
	}
	defer rows.Close()
	var out []evidence.DeviceCall
	for rows.Next() {
		var c evidence.DeviceCall
		if err := rows.Scan(&c.TrialID, &c.Round, &c.Seq, &c.LogicalMs, &c.RetryNo, &c.NextLogicalMs, &c.Kind, &c.Reason); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListValveReceipts returns every valve receipt for a trial round in sequence
// order.
func (s *SQLite) ListValveReceipts(ctx context.Context, trialID string, round int) ([]evidence.ValveReceipt, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT trial_id, round, seq, logical_ms, valve_id, position, delay_ms
		 FROM valve_receipts WHERE trial_id = ? AND round = ? ORDER BY seq ASC`, trialID, round,
	)
	if err != nil {
		return nil, fmt.Errorf("list valve receipts: %w", err)
	}
	defer rows.Close()
	var out []evidence.ValveReceipt
	for rows.Next() {
		var v evidence.ValveReceipt
		if err := rows.Scan(&v.TrialID, &v.Round, &v.Seq, &v.LogicalMs, &v.ValveID, &v.Position, &v.DelayMs); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
