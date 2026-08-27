package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"abyssal-pressure-housing-qualification/evidence"
)

func TestModel_DeviceResultReplayIdempotency(t *testing.T) {
	cases := []struct {
		name         string
		body         map[string]any
		wantCalls    int
		wantReceipts int
		wantReason   string
		wantRetries  []int
		wantConflict bool
	}{
		{
			name: "timeout with operation number",
			body: map[string]any{
				"op_no": "device-timeout-1", "logical_ms": int64(2000),
				"kind": "device", "error": "timeout",
			},
			wantCalls: 1, wantReason: "timeout", wantRetries: []int{1}, wantConflict: true,
		},
		{
			name: "stale calibration with operation number",
			body: map[string]any{
				"op_no": "device-calibration-1", "logical_ms": int64(2_000_000_000_001),
				"kind": "sample", "channel": "ch-1",
			},
			wantCalls: 1, wantReason: "calibration_stale", wantRetries: []int{1}, wantConflict: true,
		},
		{
			name: "format error with operation number",
			body: map[string]any{
				"op_no": "device-format-1", "logical_ms": int64(2100), "kind": "bogus",
			},
			wantCalls: 1, wantReason: "format_error", wantRetries: []int{1}, wantConflict: true,
		},
		{
			name: "valve mismatch with operation number",
			body: map[string]any{
				"op_no": "device-valve-mismatch-1", "logical_ms": int64(2200),
				"kind": "valve", "valve_id": "v-1", "commanded_pos": 1,
				"valve_pos": 2, "commanded_ms": int64(2190),
			},
			wantCalls: 1, wantReason: "valve_mismatch", wantRetries: []int{1}, wantConflict: true,
		},
		{
			name: "timeout without operation number keeps deterministic retries",
			body: map[string]any{
				"logical_ms": int64(2300), "kind": "device", "error": "timeout",
			},
			wantCalls: 2, wantReason: "timeout", wantRetries: []int{1, 2},
		},
		{
			name: "successful valve acknowledgement remains idempotent evidence",
			body: map[string]any{
				"op_no": "device-valve-ok-1", "logical_ms": int64(2400),
				"kind": "valve", "valve_id": "v-1", "commanded_pos": 1,
				"valve_pos": 1, "commanded_ms": int64(2390),
			},
			wantReceipts: 1, wantConflict: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newTestServer(t)

			freeze := doReq(t, s, http.MethodPost, "/api/v1/configurations/freeze", validConfigJSON())
			if freeze.Code != http.StatusOK {
				t.Fatalf("freeze status = %d, body=%s", freeze.Code, freeze.Body.String())
			}
			var snap struct {
				Digest string `json:"digest"`
			}
			if err := json.Unmarshal(freeze.Body.Bytes(), &snap); err != nil {
				t.Fatalf("decode frozen configuration: %v", err)
			}

			create := doReq(t, s, http.MethodPost, "/api/v1/trials", fmt.Sprintf(`{"config_digest":%q}`, snap.Digest))
			if create.Code != http.StatusOK {
				t.Fatalf("create trial status = %d, body=%s", create.Code, create.Body.String())
			}
			var tr struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(create.Body.Bytes(), &tr); err != nil {
				t.Fatalf("decode trial: %v", err)
			}
			startup := doReq(t, s, http.MethodPost, "/api/v1/trials/"+tr.ID+"/startup", `{"leases":[{"resource_id":"collector-1"}]}`)
			if startup.Code != http.StatusOK {
				t.Fatalf("startup status = %d, body=%s", startup.Code, startup.Body.String())
			}

			requestBody, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatalf("encode request: %v", err)
			}
			path := "/api/v1/trials/" + tr.ID + "/device-results"
			for replay := 0; replay < 2; replay++ {
				response := doReq(t, s, http.MethodPost, path, string(requestBody))
				if response.Code != http.StatusOK {
					t.Fatalf("replay %d status = %d, body=%s", replay+1, response.Code, response.Body.String())
				}
			}

			evidenceResponse := doReq(t, s, http.MethodGet, "/api/v1/trials/"+tr.ID+"/evidence", "")
			if evidenceResponse.Code != http.StatusOK {
				t.Fatalf("evidence status = %d, body=%s", evidenceResponse.Code, evidenceResponse.Body.String())
			}
			var chain struct {
				DeviceCalls   []evidence.DeviceCall   `json:"device_calls"`
				ValveReceipts []evidence.ValveReceipt `json:"valve_receipts"`
			}
			if err := json.Unmarshal(evidenceResponse.Body.Bytes(), &chain); err != nil {
				t.Fatalf("decode evidence: %v", err)
			}
			if len(chain.DeviceCalls) != tc.wantCalls {
				t.Fatalf("device call count = %d, want %d: %+v", len(chain.DeviceCalls), tc.wantCalls, chain.DeviceCalls)
			}
			if len(chain.ValveReceipts) != tc.wantReceipts {
				t.Fatalf("valve receipt count = %d, want %d: %+v", len(chain.ValveReceipts), tc.wantReceipts, chain.ValveReceipts)
			}
			for i, retryNo := range tc.wantRetries {
				if chain.DeviceCalls[i].RetryNo != retryNo || chain.DeviceCalls[i].Reason != tc.wantReason {
					t.Errorf("device call %d = retry %d reason %q, want retry %d reason %q", i, chain.DeviceCalls[i].RetryNo, chain.DeviceCalls[i].Reason, retryNo, tc.wantReason)
				}
			}

			if tc.wantConflict {
				changed := make(map[string]any, len(tc.body)+1)
				for key, value := range tc.body {
					changed[key] = value
				}
				changed["pressure_pa"] = int64(1)
				changedBody, err := json.Marshal(changed)
				if err != nil {
					t.Fatalf("encode changed request: %v", err)
				}
				conflict := doReq(t, s, http.MethodPost, path, string(changedBody))
				if conflict.Code != http.StatusConflict {
					t.Fatalf("changed digest status = %d, want %d, body=%s", conflict.Code, http.StatusConflict, conflict.Body.String())
				}
				var apiErr APIError
				if err := json.Unmarshal(conflict.Body.Bytes(), &apiErr); err != nil {
					t.Fatalf("decode conflict: %v", err)
				}
				if apiErr.Code != CodeIdempotencyConflict {
					t.Fatalf("changed digest code = %q, want %q", apiErr.Code, CodeIdempotencyConflict)
				}
			}
		})
	}
}
