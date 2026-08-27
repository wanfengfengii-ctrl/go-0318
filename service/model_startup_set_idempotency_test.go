package service

import (
	"context"
	"errors"
	"testing"

	"abyssal-pressure-housing-qualification/trial"
)

func TestModel_StartupSetIdempotency(t *testing.T) {
	type orderedRequest struct {
		OpNo  string   `json:"op_no"`
		Steps []string `json:"steps"`
	}

	reverseBindings := func(req StartupRequest) StartupRequest {
		req.Bindings = []trial.Binding{req.Bindings[1], req.Bindings[0]}
		return req
	}
	reverseLeases := func(req StartupRequest) StartupRequest {
		req.Leases = []trial.Lease{req.Leases[1], req.Leases[0]}
		return req
	}

	tests := []struct {
		name         string
		change       func(StartupRequest) StartupRequest
		wantConflict bool
		ordered      []orderedRequest
	}{
		{
			name: "reordered leases replay as the same resource set",
			change: func(req StartupRequest) StartupRequest {
				return reverseLeases(req)
			},
		},
		{
			name: "reordered bindings replay as the same binding set",
			change: func(req StartupRequest) StartupRequest {
				return reverseBindings(req)
			},
		},
		{
			name: "both startup sets may be reordered together",
			change: func(req StartupRequest) StartupRequest {
				return reverseLeases(reverseBindings(req))
			},
		},
		{
			name: "adding a resource conflicts",
			change: func(req StartupRequest) StartupRequest {
				req.Leases = append(req.Leases, trial.Lease{ResourceID: "valve-1", Holder: "holder-a", Token: "token-v", ExpiresAt: 90_000})
				return req
			},
			wantConflict: true,
		},
		{
			name: "deleting a resource conflicts",
			change: func(req StartupRequest) StartupRequest {
				req.Leases = req.Leases[:1]
				return req
			},
			wantConflict: true,
		},
		{
			name: "changing a resource conflicts",
			change: func(req StartupRequest) StartupRequest {
				req.Leases[0].ResourceID = "collector-2"
				return req
			},
			wantConflict: true,
		},
		{
			name: "adding a binding conflicts",
			change: func(req StartupRequest) StartupRequest {
				req.Bindings = append(req.Bindings, trial.Binding{Serial: "SN-C", Type: trial.ComponentValve, Position: "p-valve"})
				return req
			},
			wantConflict: true,
		},
		{
			name: "deleting a binding conflicts",
			change: func(req StartupRequest) StartupRequest {
				req.Bindings = req.Bindings[:1]
				return req
			},
			wantConflict: true,
		},
		{
			name: "changing a binding conflicts",
			change: func(req StartupRequest) StartupRequest {
				req.Bindings[0].Type = trial.ComponentValve
				return req
			},
			wantConflict: true,
		},
		{
			name: "changing a holder conflicts",
			change: func(req StartupRequest) StartupRequest {
				req.Leases[0].Holder = "holder-b"
				return req
			},
			wantConflict: true,
		},
		{
			name: "changing a token conflicts",
			change: func(req StartupRequest) StartupRequest {
				req.Leases[0].Token = "token-changed"
				return req
			},
			wantConflict: true,
		},
		{
			name: "changing expiry conflicts",
			change: func(req StartupRequest) StartupRequest {
				req.Leases[0].ExpiresAt++
				return req
			},
			wantConflict: true,
		},
		{
			name: "non-startup arrays retain ordered semantics",
			ordered: []orderedRequest{
				{OpNo: "ordered-op", Steps: []string{"pressurize", "hold"}},
				{OpNo: "ordered-op", Steps: []string{"hold", "pressurize"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.ordered) != 0 {
				first, err := trial.RequestDigest(tt.ordered[0])
				if err != nil {
					t.Fatalf("digest first ordered request: %v", err)
				}
				second, err := trial.RequestDigest(tt.ordered[1])
				if err != nil {
					t.Fatalf("digest second ordered request: %v", err)
				}
				if first == second {
					t.Fatal("reordering an ordinary request array unexpectedly preserved its digest")
				}
				return
			}

			svc := newTestService(t)
			snap := freezeTestConfig(t, svc)
			tr := createTrial(t, svc, snap.Digest)
			original := StartupRequest{
				TrialID: tr.ID,
				OpNo:    "startup-set-op",
				Bindings: []trial.Binding{
					{Serial: "SN-A", Type: trial.ComponentPressureSensor, Position: "p-sensor"},
					{Serial: "SN-B", Type: trial.ComponentTemperatureSensor, Position: "p-temp"},
				},
				Leases: []trial.Lease{
					{ResourceID: "collector-1", Holder: "holder-a", Token: "token-c", ExpiresAt: 90_000},
					{ResourceID: "pump-1", Holder: "holder-a", Token: "token-p", ExpiresAt: 90_000},
				},
			}
			if err := svc.Startup(context.Background(), original); err != nil {
				t.Fatalf("initial startup: %v", err)
			}

			replay := original
			replay.Bindings = append([]trial.Binding(nil), original.Bindings...)
			replay.Leases = append([]trial.Lease(nil), original.Leases...)
			replay = tt.change(replay)
			err := svc.Startup(context.Background(), replay)
			if tt.wantConflict {
				if !errors.Is(err, trial.ErrIdempotencyConflict) {
					t.Fatalf("changed startup replay error = %v, want %v", err, trial.ErrIdempotencyConflict)
				}
				return
			}
			if err != nil {
				t.Fatalf("set-equivalent startup replay: %v", err)
			}
		})
	}
}
