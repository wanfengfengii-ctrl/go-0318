package httpapi

import (
	"net/http"

	"abyssal-pressure-housing-qualification/service"
)

func (s *Server) handleReportAnomaly(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req service.ReportAnomalyRequest
	if !decode(w, r, &req) {
		return
	}
	req.TrialID = id
	rs, err := s.svc.ReportAnomaly(r.Context(), req)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) handleSubmitReview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req service.SubmitReviewRequest
	if !decode(w, r, &req) {
		return
	}
	req.TrialID = id
	if err := s.svc.SubmitReview(r.Context(), req); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleFinalize(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req service.FinalizeRequest
	if !decode(w, r, &req) {
		return
	}
	req.TrialID = id
	cred, err := s.svc.Finalize(r.Context(), req)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cred)
}

func (s *Server) handleTerminate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req service.TerminateRequest
	if !decode(w, r, &req) {
		return
	}
	req.TrialID = id
	t, err := s.svc.Terminate(r.Context(), req)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleGetCredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cred, err := s.svc.GetCredential(r.Context(), id)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cred)
}
