package httpapi

import (
	"net/http"

	"abyssal-pressure-housing-qualification/service"
)

func (s *Server) handleSubmitSample(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req service.SubmitSampleRequest
	if !decode(w, r, &req) {
		return
	}
	req.TrialID = id
	if err := s.svc.SubmitSample(r.Context(), req); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeviceResult(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req service.SubmitDeviceResultRequest
	if !decode(w, r, &req) {
		return
	}
	req.TrialID = id
	if err := s.svc.SubmitDeviceResult(r.Context(), req); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleGetEvidence returns the canonically ordered evidence chain for a trial:
// windows, samples, device calls, and valve receipts for the current round.
func (s *Server) handleGetEvidence(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.svc.GetTrial(r.Context(), id)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	windows, _ := s.svc.ListWindows(r.Context(), id, t.Round)
	samples, _ := s.svc.ListSamples(r.Context(), id, t.Round)
	calls, _ := s.svc.ListDeviceCalls(r.Context(), id, t.Round)
	receipts, _ := s.svc.ListValveReceipts(r.Context(), id, t.Round)
	writeJSON(w, http.StatusOK, map[string]any{
		"windows":        windows,
		"samples":        samples,
		"device_calls":   calls,
		"valve_receipts": receipts,
	})
}
