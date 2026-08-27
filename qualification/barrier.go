package qualification

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// ErrFinalStateConflict is returned when a terminal transition loses the race
// against an already-committed terminal state.
var ErrFinalStateConflict = errors.New("final state conflict")

// AdmissionPreconditions captures every condition that must hold before the
// terminal barrier may commit an admission credential.
type AdmissionPreconditions struct {
	ConfigConsistent bool
	AllStepsComplete bool
	EvidenceNoGaps   bool
	VentedToZero     bool
	ChecksPassed     bool
	RetestCleared    bool
	Reviews          []Review
}

// Validate returns a deterministic, ordered list of unmet preconditions, or
// nil when admission is permitted. Review pairs require two distinct operators
// with currently valid qualifications.
func (p AdmissionPreconditions) Validate() error {
	var unmet []string
	if !p.ConfigConsistent {
		unmet = append(unmet, "configuration mismatch")
	}
	if !p.AllStepsComplete {
		unmet = append(unmet, "steps incomplete")
	}
	if !p.EvidenceNoGaps {
		unmet = append(unmet, "evidence window gap")
	}
	if !p.VentedToZero {
		unmet = append(unmet, "pressure not vented to zero")
	}
	if !p.ChecksPassed {
		unmet = append(unmet, "visual or seal check not passed")
	}
	if !p.RetestCleared {
		unmet = append(unmet, "retest scope not cleared")
	}
	if len(p.Reviews) < 2 || !ValidateReviews(p.Reviews[0], p.Reviews[1]) {
		unmet = append(unmet, "two independent valid reviews required")
	}
	if len(unmet) > 0 {
		return fmt.Errorf("admission preconditions unmet: %v", unmet)
	}
	return nil
}

// EvidenceRoot folds a sequence of evidence digests into a single deterministic
// root digest. The root is used both to detect evidence gaps and to bind the
// admission credential to the exact evidence chain.
func EvidenceRoot(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
