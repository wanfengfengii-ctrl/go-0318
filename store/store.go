// Package store defines the persistence boundary and a SQLite implementation
// that survives process restarts. All writes are transactional; a failed write
// leaves no partial bindings, leases, stages, evidence, or credentials behind.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"

	"abyssal-pressure-housing-qualification/configuration"
	"abyssal-pressure-housing-qualification/evidence"
	"abyssal-pressure-housing-qualification/qualification"
	"abyssal-pressure-housing-qualification/trial"
)

// ErrStoreBusy is returned when the database is busy beyond the fixed retry
// budget.
var ErrStoreBusy = errors.New("store busy")

// ErrVersionConflict is returned when an optimistic-concurrency update loses
// the race against a concurrent writer.
var ErrVersionConflict = errors.New("aggregate version conflict")

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// busyRetries is the fixed retry budget for SQLITE_BUSY conditions.
const busyRetries = 5

// Store is the persistence boundary for the service.
type Store interface {
	SaveConfiguration(ctx context.Context, snap *configuration.Snapshot) error
	GetConfiguration(ctx context.Context, digest string) (*configuration.Snapshot, error)

	CreateTrial(ctx context.Context, t *trial.Trial) error
	GetTrial(ctx context.Context, id string) (*trial.Trial, error)
	UpdateTrial(ctx context.Context, id string, expectedVersion int64, mutate func(*trial.Trial) error) (*trial.Trial, error)
	AppendEvent(ctx context.Context, trialID string, e trial.Event) error
	ListEvents(ctx context.Context, trialID string) ([]trial.Event, error)
	NextSeq(ctx context.Context, trialID string) (int64, error)

	Startup(ctx context.Context, trialID string, round int, bindings []trial.Binding, leases []trial.Lease) error
	RenewLease(ctx context.Context, trialID, resourceID, holder, token string, newExpiry int64) error
	ListLeases(ctx context.Context, trialID string, round int) ([]trial.Lease, error)
	ListBindings(ctx context.Context, trialID string, round int) ([]trial.Binding, error)

	AppendSample(ctx context.Context, s evidence.Sample) error
	AppendDeviceCall(ctx context.Context, c evidence.DeviceCall) error
	AppendValveReceipt(ctx context.Context, v evidence.ValveReceipt) error
	ListSamples(ctx context.Context, trialID string, round int) ([]evidence.Sample, error)
	ListDeviceCalls(ctx context.Context, trialID string, round int) ([]evidence.DeviceCall, error)
	ListValveReceipts(ctx context.Context, trialID string, round int) ([]evidence.ValveReceipt, error)

	SaveWindow(ctx context.Context, w evidence.EvidenceWindow) error
	ListWindows(ctx context.Context, trialID string, round int) ([]evidence.EvidenceWindow, error)

	SaveRetestSet(ctx context.Context, rs qualification.RetestSet) error
	GetRetestSet(ctx context.Context, trialID string, round int) (*qualification.RetestSet, error)
	ClearRetestSet(ctx context.Context, trialID string, round int) error

	SaveReview(ctx context.Context, r qualification.Review) error
	ListReviews(ctx context.Context, trialID string, round int) ([]qualification.Review, error)

	SaveCredential(ctx context.Context, c qualification.Credential) error
	GetCredential(ctx context.Context, trialID string) (*qualification.Credential, error)
	CommitTerminal(ctx context.Context, trialID string, expectedVersion int64, ts trial.TerminalState, cred *qualification.Credential) (*trial.Trial, error)

	GetIdempotency(ctx context.Context, opNo string) (*trial.IdempotencyRecord, error)
	SaveIdempotency(ctx context.Context, rec trial.IdempotencyRecord) error

	Close() error
}

// SQLite is a Store backed by a SQLite database file.
type SQLite struct {
	db *sql.DB
}

// Open opens (or creates) a SQLite database at path, applies the schema, and
// prepares the connection for concurrent transactional use. The in-memory
// database is given a unique name so independent stores never share state.
func Open(path string) (*SQLite, error) {
	dsn := path
	if path == ":memory:" {
		dsn = fmt.Sprintf("file:memdb%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", nextMemID())
	} else {
		dsn = "file:" + path + "?_pragma=busy_timeout(1000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &SQLite{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// memSeq generates unique in-memory database names.
var memSeq atomic.Int64

func nextMemID() int64 { return memSeq.Add(1) }

func (s *SQLite) migrate() error {
	for _, stmt := range schema {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("apply schema: %w", err)
		}
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)`, schemaVersion); err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}
	return nil
}

// Close releases the database handle.
func (s *SQLite) Close() error { return s.db.Close() }

// withRetry runs fn, retrying SQLITE_BUSY up to a fixed budget before returning
// ErrStoreBusy. The retry loop keeps the failure boundary deterministic.
func withRetry(fn func() error) error {
	var err error
	for i := 0; i < busyRetries; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !isBusy(err) {
			return err
		}
		defer time.Sleep(time.Millisecond << uint(i))
	}
	return ErrStoreBusy
}

func isBusy(err error) bool {
	return err != nil && strings.Contains(err.Error(), "database is locked")
}
