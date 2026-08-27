package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"abyssal-pressure-housing-qualification/evidence"
	"abyssal-pressure-housing-qualification/qualification"
	"abyssal-pressure-housing-qualification/store"
	"abyssal-pressure-housing-qualification/trial"
)

// ErrorCode is a stable, machine-readable error identifier. Clients rely on
// these codes remaining fixed across releases.
type ErrorCode string

const (
	CodeBrokenPressurePath   ErrorCode = "BROKEN_PRESSURE_PATH"
	CodeCalibrationStale     ErrorCode = "CALIBRATION_STALE"
	CodeStepOutOfOrder       ErrorCode = "STEP_OUT_OF_ORDER"
	CodeSampleOutOfOrder     ErrorCode = "SAMPLE_OUT_OF_ORDER"
	CodeRateExceeded         ErrorCode = "RATE_EXCEEDED"
	CodeLeakLimitExceeded    ErrorCode = "LEAK_LIMIT_EXCEEDED"
	CodeLeaseExpired         ErrorCode = "LEASE_EXPIRED"
	CodeRoundStale           ErrorCode = "ROUND_STALE"
	CodeIdempotencyConflict  ErrorCode = "IDEMPOTENCY_CONFLICT"
	CodeFinalStateConflict   ErrorCode = "FINAL_STATE_CONFLICT"
	CodeOverpressure         ErrorCode = "OVERPRESSURE"
	CodeValveMismatch        ErrorCode = "VALVE_MISMATCH"
	CodeStoreBusy            ErrorCode = "STORE_BUSY"
	CodeInvalidConfiguration ErrorCode = "INVALID_CONFIGURATION"
	CodeNotFound             ErrorCode = "NOT_FOUND"
	CodeInternal             ErrorCode = "INTERNAL"
)

// APIError is the stable error envelope returned by every failing endpoint.
// Details are sorted deterministically so clients can rely on ordering.
type APIError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Details []string  `json:"details,omitempty"`
}

func (e *APIError) Error() string { return string(e.Code) + ": " + e.Message }

// mapDomainError maps a domain or persistence error to a stable error code and
// HTTP status.
func mapDomainError(err error) (ErrorCode, int) {
	switch {
	case errors.Is(err, trial.ErrIdempotencyConflict):
		return CodeIdempotencyConflict, http.StatusConflict
	case errors.Is(err, qualification.ErrFinalStateConflict):
		return CodeFinalStateConflict, http.StatusConflict
	case errors.Is(err, trial.ErrAlreadyTerminal):
		return CodeFinalStateConflict, http.StatusConflict
	case errors.Is(err, trial.ErrStepOutOfOrder):
		return CodeStepOutOfOrder, http.StatusConflict
	case errors.Is(err, evidence.ErrSampleOutOfOrder):
		return CodeSampleOutOfOrder, http.StatusConflict
	case errors.Is(err, evidence.ErrRateExceeded):
		return CodeRateExceeded, http.StatusUnprocessableEntity
	case errors.Is(err, evidence.ErrLeakLimit), errors.Is(err, evidence.ErrDropExceeded):
		return CodeLeakLimitExceeded, http.StatusUnprocessableEntity
	case errors.Is(err, evidence.ErrLeaseExpired):
		return CodeLeaseExpired, http.StatusConflict
	case errors.Is(err, evidence.ErrRoundStale):
		return CodeRoundStale, http.StatusConflict
	case errors.Is(err, evidence.ErrCalibrationStale):
		return CodeCalibrationStale, http.StatusUnprocessableEntity
	case errors.Is(err, evidence.ErrOverpressure):
		return CodeOverpressure, http.StatusUnprocessableEntity
	case errors.Is(err, evidence.ErrValveMismatch):
		return CodeValveMismatch, http.StatusConflict
	case errors.Is(err, store.ErrStoreBusy):
		return CodeStoreBusy, http.StatusServiceUnavailable
	case errors.Is(err, store.ErrNotFound):
		return CodeNotFound, http.StatusNotFound
	case errors.Is(err, store.ErrVersionConflict):
		return CodeStoreBusy, http.StatusConflict
	default:
		return CodeInternal, http.StatusInternalServerError
	}
}

// writeDomainError writes the stable error envelope for a domain error.
func writeDomainError(w http.ResponseWriter, err error) {
	code, status := mapDomainError(err)
	writeError(w, code, status, err.Error())
}

// writeError writes the stable error envelope with an appropriate HTTP status.
func writeError(w http.ResponseWriter, code ErrorCode, status int, message string, details ...string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIError{Code: code, Message: message, Details: details})
}

// writeJSON writes a successful JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
