package service

import (
	"context"
	"errors"
	"testing"

	"abyssal-pressure-housing-qualification/trial"
)

func TestModel_RestartRoundReleasesHistoricalOccupancy(t *testing.T) {
	tests := []struct {
		name           string
		restartedBind  trial.Binding
		restartedLease trial.Lease
		claimOldLease  bool
		contenderBind  trial.Binding
		contenderLease trial.Lease
	}{
		{
			name:           "serial is reusable after restart but remains exclusive while active",
			restartedBind:  trial.Binding{Serial: "sensor-old", Type: trial.ComponentPressureSensor, Position: "port-new"},
			restartedLease: trial.Lease{ResourceID: "pump-new"},
			contenderBind:  trial.Binding{Serial: "sensor-old", Type: trial.ComponentPressureSensor, Position: "port-contender"},
			contenderLease: trial.Lease{ResourceID: "pump-contender"},
		},
		{
			name:           "position is reusable after restart but remains exclusive while active",
			restartedBind:  trial.Binding{Serial: "sensor-new", Type: trial.ComponentPressureSensor, Position: "port-old"},
			restartedLease: trial.Lease{ResourceID: "pump-new"},
			contenderBind:  trial.Binding{Serial: "sensor-contender", Type: trial.ComponentPressureSensor, Position: "port-old"},
			contenderLease: trial.Lease{ResourceID: "pump-contender"},
		},
		{
			name:           "resource is reusable after restart but remains exclusive while active",
			restartedBind:  trial.Binding{Serial: "sensor-new", Type: trial.ComponentPressureSensor, Position: "port-new"},
			restartedLease: trial.Lease{ResourceID: "pump-new"},
			claimOldLease:  true,
			contenderBind:  trial.Binding{Serial: "sensor-contender", Type: trial.ComponentPressureSensor, Position: "port-contender"},
			contenderLease: trial.Lease{ResourceID: "pump-old"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			svc := newTestService(t)
			snap := freezeTestConfig(t, svc)
			original := createTrial(t, svc, snap.Digest)

			if err := svc.Startup(ctx, StartupRequest{
				TrialID: original.ID,
				Bindings: []trial.Binding{{
					Serial: "sensor-old", Type: trial.ComponentPressureSensor, Position: "port-old",
				}},
				Leases: []trial.Lease{{ResourceID: "pump-old", ExpiresAt: 10_000}},
			}); err != nil {
				t.Fatalf("start original round: %v", err)
			}
			if err := svc.SubmitSample(ctx, SubmitSampleRequest{
				TrialID: original.ID, Round: 1, LogicalMs: 2_000, PressurePa: 5_000_000, TempMC: 20_000,
			}); err != nil {
				t.Fatalf("record original-round evidence: %v", err)
			}

			restarted, err := svc.RestartRound(ctx, RestartRoundRequest{TrialID: original.ID})
			if err != nil {
				t.Fatalf("restart round: %v", err)
			}
			if restarted.Round != 2 {
				t.Fatalf("restarted round = %d, want 2", restarted.Round)
			}

			if err := svc.Startup(ctx, StartupRequest{
				TrialID: original.ID, Bindings: []trial.Binding{tt.restartedBind}, Leases: []trial.Lease{tt.restartedLease},
			}); err != nil {
				t.Fatalf("new round could not claim released occupancy: %v", err)
			}

			oldBindings, err := svc.ListBindings(ctx, original.ID, 1)
			if err != nil || len(oldBindings) != 1 || oldBindings[0].Serial != "sensor-old" || oldBindings[0].Position != "port-old" {
				t.Fatalf("old binding history not auditable: bindings=%v err=%v", oldBindings, err)
			}
			oldLeases, err := svc.ListLeases(ctx, original.ID, 1)
			if err != nil || len(oldLeases) != 1 || oldLeases[0].ResourceID != "pump-old" || oldLeases[0].Active {
				t.Fatalf("old lease history not retained as inactive: leases=%v err=%v", oldLeases, err)
			}
			oldSamples, err := svc.ListSamples(ctx, original.ID, 1)
			if err != nil || len(oldSamples) != 1 || oldSamples[0].PressurePa != 5_000_000 {
				t.Fatalf("old evidence not auditable: samples=%v err=%v", oldSamples, err)
			}

			if tt.claimOldLease {
				claimant := createTrial(t, svc, snap.Digest)
				if err := svc.Startup(ctx, StartupRequest{
					TrialID:  claimant.ID,
					Bindings: []trial.Binding{{Serial: "sensor-claimant", Type: trial.ComponentPressureSensor, Position: "port-claimant"}},
					Leases:   []trial.Lease{{ResourceID: "pump-old"}},
				}); err != nil {
					t.Fatalf("released old-round resource remained occupied: %v", err)
				}
			}

			contender := createTrial(t, svc, snap.Digest)
			err = svc.Startup(ctx, StartupRequest{
				TrialID: contender.ID, Bindings: []trial.Binding{tt.contenderBind}, Leases: []trial.Lease{tt.contenderLease},
			})
			if !errors.Is(err, ErrStartupConflict) {
				t.Fatalf("active occupancy was not rejected: got %v", err)
			}
		})
	}
}
