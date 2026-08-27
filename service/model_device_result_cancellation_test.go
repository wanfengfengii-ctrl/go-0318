package service

import (
	"context"
	"errors"
	"testing"

	"abyssal-pressure-housing-qualification/configuration"
	"abyssal-pressure-housing-qualification/evidence"
	"abyssal-pressure-housing-qualification/store"
	"abyssal-pressure-housing-qualification/trial"
)

type cancelAfterDeviceReadStore struct {
	store.Store
	cancel context.CancelFunc
	point  string
}

func (s *cancelAfterDeviceReadStore) arm(cancel context.CancelFunc, point string) {
	s.cancel = cancel
	s.point = point
}

func (s *cancelAfterDeviceReadStore) GetTrial(ctx context.Context, id string) (*trial.Trial, error) {
	tr, err := s.Store.GetTrial(ctx, id)
	if err == nil && s.point == "trial" {
		s.point = ""
		s.cancel()
	}
	return tr, err
}

func (s *cancelAfterDeviceReadStore) GetConfiguration(ctx context.Context, digest string) (*configuration.Snapshot, error) {
	snap, err := s.Store.GetConfiguration(ctx, digest)
	if err == nil && s.point == "configuration" {
		s.point = ""
		s.cancel()
	}
	return snap, err
}

func TestModel_SubmitDeviceResultCancellationStopsPersistence(t *testing.T) {
	cases := []struct {
		name           string
		request        SubmitDeviceResultRequest
		cancelPoint    string
		wantReason     string
		staleCalib     bool
		wantValve      bool
		wantValveDelay int64
	}{
		{
			name:        "timeout",
			request:     SubmitDeviceResultRequest{Kind: "device", Error: "timeout"},
			cancelPoint: "trial", wantReason: "timeout",
		},
		{
			name:        "format error",
			request:     SubmitDeviceResultRequest{Kind: "bogus"},
			cancelPoint: "trial", wantReason: "format_error",
		},
		{
			name:        "expired calibration",
			request:     SubmitDeviceResultRequest{Kind: "sample", Channel: "ch-1"},
			cancelPoint: "configuration", wantReason: "calibration_stale", staleCalib: true,
		},
		{
			name: "contradictory valve position",
			request: SubmitDeviceResultRequest{
				Kind: "valve", ValveID: "v-1", CommandedPos: 1, ValvePos: 2,
			},
			cancelPoint: "trial", wantReason: "valve_mismatch",
		},
		{
			name: "qualified valve receipt",
			request: SubmitDeviceResultRequest{
				Kind: "valve", ValveID: "v-1", CommandedPos: 1, ValvePos: 1,
			},
			wantValve: true, wantValveDelay: 50,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, err := store.Open(":memory:")
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = base.Close() })
			guarded := &cancelAfterDeviceReadStore{Store: base}
			svc := New(guarded)
			svc.SetClock(func() int64 { return testNow })

			cfg := testConfig()
			if tc.staleCalib {
				cfg.Calibrations[0].ExpiresAtMs = 500
			}
			snap, err := svc.FreezeConfiguration(context.Background(), cfg)
			if err != nil {
				t.Fatalf("freeze configuration: %v", err)
			}
			tr := createTrial(t, svc, snap.Digest)
			if err := svc.Startup(context.Background(), StartupRequest{
				TrialID: tr.ID,
				Leases:  []trial.Lease{{ResourceID: "collector-1"}},
			}); err != nil {
				t.Fatalf("startup: %v", err)
			}

			if tc.wantValve {
				before, err := base.ListEvents(context.Background(), tr.ID)
				if err != nil {
					t.Fatalf("list events before valve result: %v", err)
				}
				req := tc.request
				req.TrialID = tr.ID
				req.LogicalMs = 2050
				req.CommandedMs = 2000
				if err := svc.SubmitDeviceResult(context.Background(), req); err != nil {
					t.Fatalf("submit qualified valve result: %v", err)
				}
				receipts, err := svc.ListValveReceipts(context.Background(), tr.ID, 1)
				if err != nil {
					t.Fatalf("list valve receipts: %v", err)
				}
				if len(receipts) != 1 || receipts[0].Position != 1 || receipts[0].DelayMs != tc.wantValveDelay {
					t.Fatalf("qualified valve receipt = %+v", receipts)
				}
				calls, err := svc.ListDeviceCalls(context.Background(), tr.ID, 1)
				if err != nil {
					t.Fatalf("list device calls: %v", err)
				}
				if len(calls) != 0 {
					t.Fatalf("qualified valve produced device calls: %+v", calls)
				}
				after, err := base.ListEvents(context.Background(), tr.ID)
				if err != nil {
					t.Fatalf("list events after valve result: %v", err)
				}
				if len(after) != len(before)+1 || after[len(after)-1].Kind != trial.EventValveReceipt {
					t.Fatalf("valve events = %+v", after)
				}
				return
			}

			accepted := tc.request
			accepted.TrialID = tr.ID
			accepted.LogicalMs = 2000
			accepted.CommandedMs = 2000
			if err := svc.SubmitDeviceResult(context.Background(), accepted); err != nil {
				t.Fatalf("submit first failure: %v", err)
			}
			beforeCalls, err := svc.ListDeviceCalls(context.Background(), tr.ID, 1)
			if err != nil {
				t.Fatalf("list calls before cancellation: %v", err)
			}
			beforeEvents, err := base.ListEvents(context.Background(), tr.ID)
			if err != nil {
				t.Fatalf("list events before cancellation: %v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			guarded.arm(cancel, tc.cancelPoint)
			cancelled := tc.request
			cancelled.TrialID = tr.ID
			cancelled.LogicalMs = 2100
			cancelled.CommandedMs = 2100
			err = svc.SubmitDeviceResult(ctx, cancelled)
			cancel()
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled submission error = %v, want context.Canceled", err)
			}
			afterCancelCalls, err := svc.ListDeviceCalls(context.Background(), tr.ID, 1)
			if err != nil {
				t.Fatalf("list calls after cancellation: %v", err)
			}
			afterCancelEvents, err := base.ListEvents(context.Background(), tr.ID)
			if err != nil {
				t.Fatalf("list events after cancellation: %v", err)
			}
			if len(afterCancelCalls) != len(beforeCalls) {
				t.Fatalf("cancelled submission added a device call: before=%d after=%d", len(beforeCalls), len(afterCancelCalls))
			}
			if len(afterCancelEvents) != len(beforeEvents) {
				t.Fatalf("cancelled submission appended an event: before=%d after=%d", len(beforeEvents), len(afterCancelEvents))
			}

			accepted.LogicalMs = 2200
			accepted.CommandedMs = 2200
			if err := svc.SubmitDeviceResult(context.Background(), accepted); err != nil {
				t.Fatalf("submit failure after cancellation: %v", err)
			}
			calls, err := svc.ListDeviceCalls(context.Background(), tr.ID, 1)
			if err != nil {
				t.Fatalf("list final device calls: %v", err)
			}
			if len(calls) != 2 {
				t.Fatalf("device calls = %+v, want two accepted failures", calls)
			}
			for i, call := range calls {
				if call.RetryNo != i+1 || call.Reason != tc.wantReason {
					t.Fatalf("call %d = %+v, want retry %d reason %q", i, call, i+1, tc.wantReason)
				}
				if call.NextLogicalMs != evidence.NextRetryClock(call.LogicalMs) {
					t.Fatalf("call %d next logical time = %d", i, call.NextLogicalMs)
				}
			}
			if calls[1].Seq != calls[0].Seq+2 {
				t.Fatalf("cancelled submission consumed an evidence sequence: call seqs %d and %d", calls[0].Seq, calls[1].Seq)
			}
			finalEvents, err := base.ListEvents(context.Background(), tr.ID)
			if err != nil {
				t.Fatalf("list final events: %v", err)
			}
			if len(finalEvents) != len(beforeEvents)+1 || finalEvents[len(finalEvents)-1].Kind != trial.EventDeviceCall {
				t.Fatalf("final device-call events = %+v", finalEvents)
			}
		})
	}
}
