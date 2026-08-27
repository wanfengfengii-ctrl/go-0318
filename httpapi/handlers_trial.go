package httpapi

import (
	"net/http"
	"strconv"

	"abyssal-pressure-housing-qualification/service"
)

func (s *Server) handleCreateTrial(w http.ResponseWriter, r *http.Request) {
	var req service.CreateTrialRequest
	if !decode(w, r, &req) {
		return
	}
	t, err := s.svc.CreateTrial(r.Context(), req)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleGetTrial(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.svc.GetTrial(r.Context(), id)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	leases, _ := s.svc.ListLeases(r.Context(), id, t.Round)
	bindings, _ := s.svc.ListBindings(r.Context(), id, t.Round)
	retest, _ := s.svc.GetRetestSet(r.Context(), id, t.Round)
	writeJSON(w, http.StatusOK, map[string]any{
		"trial":      t,
		"leases":     leases,
		"bindings":   bindings,
		"retest_set": retest,
	})
}

func (s *Server) handleStartup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req service.StartupRequest
	if !decode(w, r, &req) {
		return
	}
	req.TrialID = id
	if err := s.svc.Startup(r.Context(), req); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleRenewLease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req service.RenewLeaseRequest
	if !decode(w, r, &req) {
		return
	}
	req.TrialID = id
	if err := s.svc.RenewLease(r.Context(), req); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdvanceStage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	stage := r.PathValue("stage")
	req := service.StageRequest{TrialID: id, Stage: stage}
	t, err := s.svc.AdvanceStage(r.Context(), req)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleCompleteStep(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	step, err := strconv.Atoi(r.PathValue("step"))
	if err != nil {
		writeError(w, CodeInvalidConfiguration, http.StatusBadRequest, "invalid step index")
		return
	}
	var req service.CompleteStepRequest
	if !decode(w, r, &req) {
		return
	}
	req.TrialID = id
	req.StepIndex = step
	t, err := s.svc.CompleteStep(r.Context(), req)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleRestartRound(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req service.RestartRoundRequest
	if !decode(w, r, &req) {
		return
	}
	req.TrialID = id
	t, err := s.svc.RestartRound(r.Context(), req)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}
