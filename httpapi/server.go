// Package httpapi exposes the versioned JSON API, the stable error protocol,
// and the operator frontend. It performs only parsing, operator-identity
// capture, and stable error mapping; every business rule lives in the service
// and domain packages.
package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"abyssal-pressure-housing-qualification/service"
)

// Server wires the HTTP API to the application service and serves the operator
// frontend.
type Server struct {
	svc         *service.Service
	frontendDir string
	mux         *http.ServeMux
}

// New constructs an HTTP server. frontendDir may be empty; when it names an
// existing directory its built page is served, otherwise an embedded fallback
// page is used.
func New(svc *service.Service, frontendDir string) *Server {
	s := &Server{svc: svc, frontendDir: frontendDir, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	mux := s.mux
	mux.HandleFunc("POST /api/v1/configurations/freeze", s.handleFreeze)
	mux.HandleFunc("GET /api/v1/configurations/{digest}", s.handleGetConfiguration)

	mux.HandleFunc("POST /api/v1/trials", s.handleCreateTrial)
	mux.HandleFunc("GET /api/v1/trials/{id}", s.handleGetTrial)
	mux.HandleFunc("POST /api/v1/trials/{id}/startup", s.handleStartup)
	mux.HandleFunc("POST /api/v1/trials/{id}/leases/renew", s.handleRenewLease)
	mux.HandleFunc("POST /api/v1/trials/{id}/samples", s.handleSubmitSample)
	mux.HandleFunc("POST /api/v1/trials/{id}/device-results", s.handleDeviceResult)
	mux.HandleFunc("POST /api/v1/trials/{id}/steps/{step}/complete", s.handleCompleteStep)
	mux.HandleFunc("POST /api/v1/trials/{id}/stages/{stage}", s.handleAdvanceStage)
	mux.HandleFunc("POST /api/v1/trials/{id}/anomalies", s.handleReportAnomaly)
	mux.HandleFunc("POST /api/v1/trials/{id}/restart", s.handleRestartRound)
	mux.HandleFunc("GET /api/v1/trials/{id}/evidence", s.handleGetEvidence)
	mux.HandleFunc("POST /api/v1/trials/{id}/reviews", s.handleSubmitReview)
	mux.HandleFunc("POST /api/v1/trials/{id}/finalize", s.handleFinalize)
	mux.HandleFunc("POST /api/v1/trials/{id}/terminate", s.handleTerminate)
	mux.HandleFunc("GET /api/v1/trials/{id}/credential", s.handleGetCredential)

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("/", s.handleFrontend)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// decode decodes a JSON request body, returning false (after writing an error)
// when the body is malformed.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, CodeInvalidConfiguration, http.StatusBadRequest, "malformed request body")
		return false
	}
	return true
}

// fallbackPage is the embedded operator entry page served when no built
// frontend is available. It calls the backend's health and freeze endpoints.
const fallbackPage = `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><title>深海采样器压力舱耐压验证</title></head>
<body>
<h1>深海采样器压力舱耐压验证</h1>
<p id="status">连接中…</p>
<script>
fetch('/health').then(r => r.json()).then(d => {
  document.getElementById('status').textContent = '后端状态: ' + JSON.stringify(d);
});
</script>
</body>
</html>`

func (s *Server) handleFrontend(w http.ResponseWriter, r *http.Request) {
	dist := ""
	if s.frontendDir != "" {
		dir := filepath.Join(s.frontendDir, "dist")
		if info, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !info.IsDir() {
			dist = dir
		}
	}
	if dist == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(fallbackPage))
		return
	}
	http.FileServer(http.Dir(dist)).ServeHTTP(w, r)
}
