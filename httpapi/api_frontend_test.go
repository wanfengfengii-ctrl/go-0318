package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"abyssal-pressure-housing-qualification/service"
	"abyssal-pressure-housing-qualification/store"
)

func newTestServer(t *testing.T) (*Server, *service.Service) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	svc := service.New(st)
	return New(svc, ""), svc
}

func doReq(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func validConfigJSON() string {
	return `{
		"chambers": [
			{"id":"c-main","name":"主承压舱段","volume_ul":1000},
			{"id":"c-end","name":"端盖舱段","volume_ul":500}
		],
		"ports": [
			{"id":"p-inlet","chamber":"c-main","kind":"pressure_inlet"},
			{"id":"p-sensor","chamber":"c-main","kind":"pressure_sensor","channel":"ch-1"},
			{"id":"p-temp","chamber":"c-end","kind":"temperature_sensor","channel":"ch-2"}
		],
		"pipes": [{"id":"pipe-1","from":"p-sensor","to":"p-temp"}],
		"seal_boundaries": [{"id":"s-1","chamber":"c-main","checks":["外观检查","密封复查"]}],
		"steps": [
			{"index":1,"target_pa":5000000,"ramp_up_pa_per_s":100000,"ramp_down_pa_per_s":100000,"hold_ms":600000,"leak_limit_ul_per_s":10,"max_drop_pa":50000},
			{"index":2,"target_pa":10000000,"ramp_up_pa_per_s":100000,"ramp_down_pa_per_s":100000,"hold_ms":600000,"leak_limit_ul_per_s":10,"max_drop_pa":50000}
		],
		"calibrations": [
			{"channel":"ch-1","serial":"SN-P","expires_at_ms":2000000000000,"summary":"压力"},
			{"channel":"ch-2","serial":"SN-T","expires_at_ms":2000000000000,"summary":"温度"}
		],
		"compensation": {"ref_temp_mc":20000,"temp_coeff_ppm":10}
	}`
}

func TestAPIStableErrorCodes(t *testing.T) {
	s, _ := newTestServer(t)

	// Missing pressure inlet maps to BROKEN_PRESSURE_PATH.
	body := strings.Replace(validConfigJSON(), `"kind":"pressure_inlet"`, `"kind":"pressure_sensor"`, 1)
	rec := doReq(t, s, http.MethodPost, "/api/v1/configurations/freeze", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var apiErr APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if apiErr.Code != CodeBrokenPressurePath {
		t.Fatalf("code = %q, want %q", apiErr.Code, CodeBrokenPressurePath)
	}

	// Uncalibrated sensor maps to CALIBRATION_STALE.
	body = strings.Replace(validConfigJSON(), `"channel":"ch-1"`, `"channel":"ch-9"`, 1)
	rec = doReq(t, s, http.MethodPost, "/api/v1/configurations/freeze", body)
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if apiErr.Code != CodeCalibrationStale {
		t.Fatalf("code = %q, want %q", apiErr.Code, CodeCalibrationStale)
	}
}

func TestAPITrialLifecycle(t *testing.T) {
	s, _ := newTestServer(t)

	rec := doReq(t, s, http.MethodPost, "/api/v1/configurations/freeze", validConfigJSON())
	if rec.Code != http.StatusOK {
		t.Fatalf("freeze status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var snap struct {
		Digest string `json:"digest"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	rec = doReq(t, s, http.MethodPost, "/api/v1/trials", `{"config_digest":"`+snap.Digest+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create trial status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var tr struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tr); err != nil {
		t.Fatalf("unmarshal trial: %v", err)
	}

	rec = doReq(t, s, http.MethodGet, "/api/v1/trials/"+tr.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get trial status = %d", rec.Code)
	}

	startup := `{"bindings":[{"serial":"SN-P","type":"pressure_sensor","position":"p-sensor"}],"leases":[{"resource_id":"collector-1"}]}`
	rec = doReq(t, s, http.MethodPost, "/api/v1/trials/"+tr.ID+"/startup", startup)
	if rec.Code != http.StatusOK {
		t.Fatalf("startup status = %d, body=%s", rec.Code, rec.Body.String())
	}

	sample := `{"logical_ms":2000,"pressure_pa":5000000,"temp_mc":20000}`
	rec = doReq(t, s, http.MethodPost, "/api/v1/trials/"+tr.ID+"/samples", sample)
	if rec.Code != http.StatusOK {
		t.Fatalf("sample status = %d, body=%s", rec.Code, rec.Body.String())
	}

	rec = doReq(t, s, http.MethodGet, "/api/v1/trials/"+tr.ID+"/evidence", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("evidence status = %d", rec.Code)
	}
}

func TestFrontendServedByGo(t *testing.T) {
	s, _ := newTestServer(t)
	rec := doReq(t, s, http.MethodGet, "/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("frontend status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "/health") {
		t.Fatal("expected frontend page to reference the backend API")
	}
}
