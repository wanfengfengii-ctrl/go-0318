package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"abyssal-pressure-housing-qualification/trial"
)

func TestModel_StageAdvanceHTTPIdempotencyProtocol(t *testing.T) {
	type request struct {
		stage      string
		body       string
		wantStatus int
	}
	tests := []struct {
		name              string
		requests          []request
		wantStage         trial.Stage
		wantVersion       int64
		wantSameResponses bool
		wantRecoverable   bool
	}{
		{
			name: "op_no retry returns the original success and the path stage is authoritative",
			requests: []request{
				{stage: "fill_vent", body: `{"trial_id":"body-id-must-not-win","stage":"admission","op_no":"advance-001"}`, wantStatus: http.StatusOK},
				{stage: "fill_vent", body: `{"trial_id":"body-id-must-not-win","stage":"admission","op_no":"advance-001"}`, wantStatus: http.StatusOK},
			},
			wantStage:         trial.StageFillVent,
			wantVersion:       1,
			wantSameResponses: true,
			wantRecoverable:   true,
		},
		{
			name: "bodyless request remains supported without idempotency",
			requests: []request{
				{stage: "fill_vent", wantStatus: http.StatusOK},
				{stage: "fill_vent", wantStatus: http.StatusInternalServerError},
			},
			wantStage:   trial.StageFillVent,
			wantVersion: 1,
		},
		{
			name: "skip is rejected",
			requests: []request{
				{stage: "step_ramp", body: `{"op_no":"advance-skip"}`, wantStatus: http.StatusInternalServerError},
			},
			wantStage:   trial.StagePrecheck,
			wantVersion: 0,
		},
		{
			name: "backward move is rejected",
			requests: []request{
				{stage: "fill_vent", wantStatus: http.StatusOK},
				{stage: "precheck", body: `{"op_no":"advance-backward"}`, wantStatus: http.StatusInternalServerError},
			},
			wantStage:   trial.StageFillVent,
			wantVersion: 1,
		},
		{
			name: "unknown stage is rejected",
			requests: []request{
				{stage: "not_a_stage", body: `{"op_no":"advance-invalid"}`, wantStatus: http.StatusInternalServerError},
			},
			wantStage:   trial.StagePrecheck,
			wantVersion: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, svc := newTestServer(t)
			freeze := doReq(t, s, http.MethodPost, "/api/v1/configurations/freeze", validConfigJSON())
			if freeze.Code != http.StatusOK {
				t.Fatalf("freeze status = %d, body=%s", freeze.Code, freeze.Body.String())
			}
			var snapshot struct {
				Digest string `json:"digest"`
			}
			if err := json.Unmarshal(freeze.Body.Bytes(), &snapshot); err != nil {
				t.Fatalf("decode frozen configuration: %v", err)
			}

			created := doReq(t, s, http.MethodPost, "/api/v1/trials", `{"config_digest":"`+snapshot.Digest+`"}`)
			if created.Code != http.StatusOK {
				t.Fatalf("create status = %d, body=%s", created.Code, created.Body.String())
			}
			var initial trial.Trial
			if err := json.Unmarshal(created.Body.Bytes(), &initial); err != nil {
				t.Fatalf("decode created trial: %v", err)
			}

			responses := make([][]byte, 0, len(tc.requests))
			for i, call := range tc.requests {
				path := "/api/v1/trials/" + initial.ID + "/stages/" + call.stage
				response := doReq(t, s, http.MethodPost, path, call.body)
				if response.Code != call.wantStatus {
					t.Fatalf("request %d status = %d, want %d, body=%s", i+1, response.Code, call.wantStatus, response.Body.String())
				}
				responses = append(responses, append([]byte(nil), response.Body.Bytes()...))
			}

			if tc.wantSameResponses && !bytes.Equal(responses[0], responses[1]) {
				t.Fatalf("retry response differs from original:\nfirst: %s\nretry: %s", responses[0], responses[1])
			}

			current, err := svc.GetTrial(context.Background(), initial.ID)
			if err != nil {
				t.Fatalf("get trial after requests: %v", err)
			}
			if current.Stage != tc.wantStage || current.Version != tc.wantVersion {
				t.Fatalf("trial state = stage %q version %d, want stage %q version %d", current.Stage, current.Version, tc.wantStage, tc.wantVersion)
			}
			if tc.wantRecoverable {
				recovered, err := svc.RecoverTrial(context.Background(), initial.ID)
				if err != nil {
					t.Fatalf("recover event stream after retry: %v", err)
				}
				if recovered.Stage != tc.wantStage {
					t.Fatalf("recovered stage = %q, want %q", recovered.Stage, tc.wantStage)
				}
			}
		})
	}
}
