package httpapi

import (
	"net/http"
	"sort"
	"strings"

	"abyssal-pressure-housing-qualification/configuration"
)

func (s *Server) handleFreeze(w http.ResponseWriter, r *http.Request) {
	var in configuration.Input
	if !decode(w, r, &in) {
		return
	}
	snap, err := s.svc.FreezeConfiguration(r.Context(), in)
	if err != nil {
		s.writeConfigError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleGetConfiguration(w http.ResponseWriter, r *http.Request) {
	digest := r.PathValue("digest")
	snap, err := s.svc.GetConfiguration(r.Context(), digest)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// writeConfigError maps a configuration validation error to a stable code,
// sorting the reasons so clients can rely on deterministic detail ordering.
func (s *Server) writeConfigError(w http.ResponseWriter, err error) {
	ve, ok := err.(*configuration.ValidationError)
	if !ok {
		writeError(w, CodeInvalidConfiguration, http.StatusBadRequest, err.Error())
		return
	}
	details := append([]string(nil), ve.Reasons...)
	sort.Strings(details)
	code := CodeInvalidConfiguration
	joined := strings.Join(details, " ")
	switch {
	case strings.Contains(joined, "pressure inlet"), strings.Contains(joined, "isolated chamber"), strings.Contains(joined, "broken pressure path"):
		code = CodeBrokenPressurePath
	case strings.Contains(joined, "uncalibrated"):
		code = CodeCalibrationStale
	}
	writeError(w, code, http.StatusBadRequest, ve.Error(), details...)
}
