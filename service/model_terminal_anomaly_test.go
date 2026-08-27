package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"abyssal-pressure-housing-qualification/trial"
)

func TestModel_AnomalyCannotMutateTerminalTrial(t *testing.T) {
	cases := []struct {
		name     string
		terminal trial.TerminalState
	}{
		{name: "after finalize", terminal: trial.TerminalAdmitted},
		{name: "after terminate", terminal: trial.TerminalTerminated},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			_, tr := startTrial(t, svc)

			switch tc.terminal {
			case trial.TerminalAdmitted:
				completeStepsAndStages(t, svc, tr)
				submitReview(t, svc, tr, "alice", 2_000_000_000_000)
				submitReview(t, svc, tr, "bob", 2_000_000_000_000)
				if _, err := svc.Finalize(context.Background(), FinalizeRequest{TrialID: tr.ID}); err != nil {
					t.Fatalf("finalize: %v", err)
				}
			case trial.TerminalTerminated:
				if _, err := svc.Terminate(context.Background(), TerminateRequest{TrialID: tr.ID}); err != nil {
					t.Fatalf("terminate: %v", err)
				}
			}

			beforeSet, err := svc.GetRetestSet(context.Background(), tr.ID, tr.Round)
			if err != nil {
				t.Fatalf("get retest set before late anomaly: %v", err)
			}
			beforeEvents, err := svc.store.ListEvents(context.Background(), tr.ID)
			if err != nil {
				t.Fatalf("list events before late anomaly: %v", err)
			}

			_, err = svc.ReportAnomaly(context.Background(), ReportAnomalyRequest{
				TrialID: tr.ID,
				Kind:    "overpressure",
				PortID:  "p-sensor",
				OpNo:    "late-anomaly-" + string(tc.terminal),
			})
			if !errors.Is(err, trial.ErrAlreadyTerminal) {
				t.Fatalf("late anomaly error = %v, want %v", err, trial.ErrAlreadyTerminal)
			}

			afterSet, err := svc.GetRetestSet(context.Background(), tr.ID, tr.Round)
			if err != nil {
				t.Fatalf("get retest set after late anomaly: %v", err)
			}
			if !reflect.DeepEqual(afterSet, beforeSet) {
				t.Fatalf("late anomaly changed retest set: before=%#v after=%#v", beforeSet, afterSet)
			}
			afterEvents, err := svc.store.ListEvents(context.Background(), tr.ID)
			if err != nil {
				t.Fatalf("list events after late anomaly: %v", err)
			}
			if !reflect.DeepEqual(afterEvents, beforeEvents) {
				t.Fatalf("late anomaly changed event stream: before=%d events after=%d events", len(beforeEvents), len(afterEvents))
			}

			got, err := svc.GetTrial(context.Background(), tr.ID)
			if err != nil {
				t.Fatalf("get terminal trial: %v", err)
			}
			if got.Terminal != tc.terminal {
				t.Fatalf("terminal = %q, want %q", got.Terminal, tc.terminal)
			}
		})
	}
}
