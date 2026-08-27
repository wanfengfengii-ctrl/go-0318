package trial

import "fmt"

// Errors describing invalid aggregate transitions.
var (
	ErrStepOutOfOrder  = fmt.Errorf("step out of order")
	ErrAlreadyTerminal = fmt.Errorf("trial already terminal")
	ErrRoundInProgress = fmt.Errorf("round still in progress")
)

// NewTrial returns a fresh trial aggregate in the precheck stage, round one,
// with the first pressure step pending.
func NewTrial(id, configDigest string, stepsTotal int) *Trial {
	return &Trial{
		ID:           id,
		ConfigDigest: configDigest,
		Stage:        StagePrecheck,
		StepIndex:    1,
		StepsTotal:   stepsTotal,
		Round:        1,
		Terminal:     TerminalNone,
		Version:      0,
	}
}

// AdvanceStage moves the trial to next, preserving the continuous-prefix
// invariant. It rejects backward movement, skips, and unknown stages.
func (t *Trial) AdvanceStage(next Stage) error {
	if t.Terminal != TerminalNone {
		return ErrAlreadyTerminal
	}
	if !CanAdvance(t.Stage, next) {
		return &StageTransitionError{From: t.Stage, To: next}
	}
	t.Stage = next
	return nil
}

// CompleteStep records completion of the pressure step with the given one-based
// index. Steps must be completed in order and may not exceed the configured
// ladder. Completing the final step does not advance the stage; that is a
// separate, explicit operation.
func (t *Trial) CompleteStep(index int) error {
	if t.Terminal != TerminalNone {
		return ErrAlreadyTerminal
	}
	if index != t.StepIndex {
		return fmt.Errorf("%w: expected step %d, got %d", ErrStepOutOfOrder, t.StepIndex, index)
	}
	if index > t.StepsTotal {
		return fmt.Errorf("%w: step %d exceeds total %d", ErrStepOutOfOrder, index, t.StepsTotal)
	}
	t.StepIndex++
	return nil
}

// AllStepsComplete reports whether every graded pressure step has been
// completed for the current round.
func (t *Trial) AllStepsComplete() bool {
	return t.StepIndex > t.StepsTotal
}

// NewRound starts a fresh round after reassembly: the round number increments,
// the stage resets to precheck and the step ladder restarts. Old rounds remain
// read-only and are never mutated again.
func (t *Trial) NewRound() error {
	if t.Terminal != TerminalNone {
		return ErrAlreadyTerminal
	}
	t.Round++
	t.Stage = StagePrecheck
	t.StepIndex = 1
	return nil
}

// SetTerminal commits the single final outcome. The terminal barrier permits
// exactly one transition from the non-terminal state.
func (t *Trial) SetTerminal(ts TerminalState) error {
	if t.Terminal != TerminalNone {
		return fmt.Errorf("%w: already %q", ErrAlreadyTerminal, t.Terminal)
	}
	switch ts {
	case TerminalAdmitted, TerminalRetest, TerminalTerminated:
		t.Terminal = ts
		return nil
	default:
		return fmt.Errorf("invalid terminal state %q", ts)
	}
}
