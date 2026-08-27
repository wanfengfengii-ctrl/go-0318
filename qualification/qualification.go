// Package qualification implements anomaly retest scoping and the admission
// arbitration: retest ranges, independent reviews, the terminal barrier, and
// the deterministic admission credential.
package qualification

import (
	"crypto/sha256"
	"encoding/hex"
)

// AnomalyKind classifies an isolation-triggering anomaly.
type AnomalyKind string

const (
	AnomalyOverpressure  AnomalyKind = "overpressure"
	AnomalyPressureDrop  AnomalyKind = "pressure_drop"
	AnomalySealLeak      AnomalyKind = "seal_leak"
	AnomalyValveMismatch AnomalyKind = "valve_mismatch"
)

// RetestMember is one de-duplicated, canonically ordered retest item.
type RetestMember struct {
	Chamber   string `json:"chamber"`
	PortID    string `json:"port_id"`
	CheckType string `json:"check_type"`
}

// RetestSet is the propagation result of one or more anomalies. Version is an
// optimistic-concurrency token: concurrent anomaly reports each read-modify-write
// the set, and a save whose Version no longer matches the stored value is
// rejected as a version conflict so the loser re-reads and re-merges instead of
// clobbering the winner's scope.
type RetestSet struct {
	TrialID string         `json:"trial_id"`
	Round   int            `json:"round"`
	Version int64          `json:"version"`
	Members []RetestMember `json:"members"`
}

// Review is an independent review submitted by an operator with a
// qualification that must be valid at review time.
type Review struct {
	TrialID       string `json:"trial_id"`
	Round         int    `json:"round"`
	Operator      string `json:"operator"`
	Qualification string `json:"qualification"`
	ValidAtMs     int64  `json:"valid_at_ms"`
	QualExpiresAt int64  `json:"qual_expires_at"`
}

// Credential is the unique voyage admission credential derived from the
// configuration digest, trial, and evidence root digest.
type Credential struct {
	TrialID    string `json:"trial_id"`
	Digest     string `json:"digest"`
	IssuedAtMs int64  `json:"issued_at_ms"`
}

// DeriveCredential deterministically derives the admission credential digest
// from the configuration digest, trial id, and evidence root digest. The same
// inputs always produce the same credential, so it remains verifiable after a
// process restart.
func DeriveCredential(configDigest, trialID, evidenceRoot string) string {
	h := sha256.New()
	h.Write([]byte(configDigest))
	h.Write([]byte{0})
	h.Write([]byte(trialID))
	h.Write([]byte{0})
	h.Write([]byte(evidenceRoot))
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateReviews reports whether two reviews form a valid independent pair:
// two different operators, each with a non-empty qualification that has not
// expired at review time.
func ValidateReviews(a, b Review) bool {
	return a.Operator != "" && b.Operator != "" &&
		a.Qualification != "" && b.Qualification != "" &&
		a.Operator != b.Operator &&
		a.ValidAtMs <= a.QualExpiresAt &&
		b.ValidAtMs <= b.QualExpiresAt
}
