// Package trial implements the pressure-trial aggregate: the fixed stage state
// machine, pressure-step progression, round identity, terminal states, and the
// append-only event stream used to rebuild materialised state after restart.
package trial

import "fmt"

// Stage is one fixed phase of a pressure trial. Stages form a continuous
// prefix: a trial may only advance to the next stage, never skip one.
type Stage string

const (
	StagePrecheck       Stage = "precheck"
	StageFillVent       Stage = "fill_vent"
	StageStepRamp       Stage = "step_ramp"
	StageHold           Stage = "hold"
	StageControlledVent Stage = "controlled_vent"
	StageRepressurize   Stage = "repressurize"
	StageVisualCheck    Stage = "visual_check"
	StageSealCheck      Stage = "seal_check"
	StageReview         Stage = "review"
	StageAdmission      Stage = "admission"
)

// stageOrder is the canonical, fixed stage sequence.
var stageOrder = []Stage{
	StagePrecheck,
	StageFillVent,
	StageStepRamp,
	StageHold,
	StageControlledVent,
	StageRepressurize,
	StageVisualCheck,
	StageSealCheck,
	StageReview,
	StageAdmission,
}

// TerminalState is the single final outcome of a trial. The terminal barrier
// guarantees exactly one of these is ever committed.
type TerminalState string

const (
	TerminalNone       TerminalState = ""
	TerminalAdmitted   TerminalState = "admitted"
	TerminalRetest     TerminalState = "retest"
	TerminalTerminated TerminalState = "terminated"
)

// Trial is the materialised state of a pressure trial aggregate.
type Trial struct {
	ID           string        `json:"id"`
	ConfigDigest string        `json:"config_digest"`
	Stage        Stage         `json:"stage"`
	StepIndex    int           `json:"step_index"`  // one-based index of the current pressure step (1 = first)
	StepsTotal   int           `json:"steps_total"` // total number of graded pressure steps
	Round        int           `json:"round"`
	Terminal     TerminalState `json:"terminal"`
	Version      int64         `json:"version"` // optimistic-concurrency version
}

// StageIndex returns the zero-based position of s in the fixed stage order,
// or -1 if s is not a known stage.
func StageIndex(s Stage) int {
	for i, st := range stageOrder {
		if st == s {
			return i
		}
	}
	return -1
}

// CanAdvance reports whether the trial may move from current to next while
// preserving the continuous-prefix invariant. Advancing to an earlier stage or
// skipping a stage is rejected.
func CanAdvance(current, next Stage) bool {
	ci, ni := StageIndex(current), StageIndex(next)
	if ci < 0 || ni < 0 {
		return false
	}
	return ni == ci+1
}

// ValidStage reports whether s is a known stage.
func ValidStage(s Stage) bool { return StageIndex(s) >= 0 }

// String returns the string form of the stage.
func (s Stage) String() string { return string(s) }

// StageTransitionError describes an invalid stage advancement.
type StageTransitionError struct {
	From Stage
	To   Stage
}

func (e *StageTransitionError) Error() string {
	return fmt.Sprintf("invalid stage transition %q -> %q", e.From, e.To)
}
