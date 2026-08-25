package httpapi

import (
	"net/http"
	"tapemastergate/internal/application"
)

type Server struct {
	app *application.Service
	mux *http.ServeMux
}

func New(app *application.Service) *Server {
	s := &Server{app: app, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return Security(Recover(RequestID(LimitBody(s.mux)))) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/v1/health", s.HandleHealth)
	s.mux.HandleFunc("GET /api/v1/jobs", s.HandleJobs)
	s.mux.HandleFunc("POST /api/v1/jobs", s.HandleJobs)
	s.mux.HandleFunc("GET /api/v1/jobs/{jobId}", s.HandleJobDetail)
	s.mux.HandleFunc("GET /api/v1/jobs/{jobId}/readiness", s.HandleReadiness)
	s.mux.HandleFunc("POST /api/v1/jobs/{jobId}/carriers", s.HandleAddCarrier)
	s.mux.HandleFunc("POST /api/v1/jobs/{jobId}/preflight", s.HandlePreflight)
	s.mux.HandleFunc("POST /api/v1/jobs/{jobId}/captures", s.HandleCapture)
	s.mux.HandleFunc("POST /api/v1/jobs/{jobId}/captures/{captureId}/void", s.HandleVoidCapture)
	s.mux.HandleFunc("GET /api/v1/jobs/{jobId}/findings", s.HandleFindings)
	s.mux.HandleFunc("POST /api/v1/jobs/{jobId}/findings", s.HandleFindings)
	s.mux.HandleFunc("POST /api/v1/jobs/{jobId}/findings/{findingId}/review", s.HandleReviewFinding)
	s.mux.HandleFunc("POST /api/v1/jobs/{jobId}/submit", s.HandleSubmitApproval)
	s.mux.HandleFunc("GET /api/v1/jobs/{jobId}/manifest/preview", s.HandleManifestPreview)
	s.mux.HandleFunc("GET /api/v1/jobs/{jobId}/manifest/preflight", s.HandleManifestPreview)
	s.mux.HandleFunc("POST /api/v1/jobs/{jobId}/manifest/freeze", s.HandleFreezeManifest)
	s.mux.HandleFunc("POST /api/v1/jobs/{jobId}/certificate", s.HandleIssueCertificate)
	s.mux.HandleFunc("GET /api/v1/jobs/{jobId}/audit", s.HandleAudit)
	s.mux.HandleFunc("GET /api/v1/certificates/{certificateNo}/verify", s.HandleVerifyCertificate)
}
