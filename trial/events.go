package trial

import (
	"encoding/json"
	"fmt"
)

// EventKind classifies an append-only trial event. Events are monotonically
// sequenced and never mutated; materialised state can be rebuilt by replaying
// the committed event stream.
type EventKind string

const (
	EventTrialCreated     EventKind = "trial_created"
	EventStageAdvanced    EventKind = "stage_advanced"
	EventStepCompleted    EventKind = "step_completed"
	EventRoundStarted     EventKind = "round_started"
	EventTerminal         EventKind = "terminal"
	EventBindingApplied   EventKind = "binding_applied"
	EventLeaseGranted     EventKind = "lease_granted"
	EventSampleAppended   EventKind = "sample_appended"
	EventDeviceCall       EventKind = "device_call"
	EventValveReceipt     EventKind = "valve_receipt"
	EventEvidenceWindow   EventKind = "evidence_window"
	EventRetestReported   EventKind = "retest_reported"
	EventReviewSubmitted  EventKind = "review_submitted"
	EventCredentialIssued EventKind = "credential_issued"
)

// Event is an append-only, monotonically sequenced trial event.
type Event struct {
	Seq     int64
	Round   int
	Kind    EventKind
	Payload []byte
}

// NewEvent marshals a payload into a trial event with the given sequence,
// round, and kind.
func NewEvent(seq int64, round int, kind EventKind, payload any) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("marshal event payload: %w", err)
	}
	return Event{Seq: seq, Round: round, Kind: kind, Payload: raw}, nil
}

// Replay rebuilds the materialised aggregate state from a committed,
// sequence-ordered event stream. It returns a fresh Trial reflecting every
// state-transition event and ignores append-only evidence events, which are
// reconstructed separately by the store.
func Replay(t *Trial, events []Event) error {
	for _, e := range events {
		switch e.Kind {
		case EventTrialCreated:
			var p struct {
				ID           string `json:"id"`
				ConfigDigest string `json:"config_digest"`
				StepsTotal   int    `json:"steps_total"`
			}
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				return err
			}
			*t = *NewTrial(p.ID, p.ConfigDigest, p.StepsTotal)
		case EventStageAdvanced:
			var p struct {
				Stage Stage `json:"stage"`
			}
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				return err
			}
			if err := t.AdvanceStage(p.Stage); err != nil {
				return err
			}
		case EventStepCompleted:
			var p struct {
				Index int `json:"index"`
			}
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				return err
			}
			if err := t.CompleteStep(p.Index); err != nil {
				return err
			}
		case EventRoundStarted:
			if err := t.NewRound(); err != nil {
				return err
			}
		case EventTerminal:
			var p struct {
				Terminal TerminalState `json:"terminal"`
			}
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				return err
			}
			if err := t.SetTerminal(p.Terminal); err != nil {
				return err
			}
		}
	}
	return nil
}
