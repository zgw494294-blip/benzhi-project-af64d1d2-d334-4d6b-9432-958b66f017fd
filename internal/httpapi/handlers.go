package httpapi

import (
	"fmt"
	"net/http"
	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
)

func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "TapeMaster Gate"})
}
func (s *Server) HandleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"jobs": s.app.ListJobs(r.Context())})
		return
	}
	var c application.CreateJobCommand
	if !decode(w, r, &c) {
		return
	}
	result, err := s.app.CreateJob(r.Context(), c)
	if err != nil {
		handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (s *Server) HandleJobDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathRequired(w, r, "jobId")
	if !ok {
		return
	}
	v, err := s.app.GetJob(r.Context(), id)
	if err != nil {
		handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) HandleReadiness(w http.ResponseWriter, r *http.Request) {
	id, ok := pathRequired(w, r, "jobId")
	if !ok {
		return
	}
	v, err := s.app.Readiness(r.Context(), id)
	if err != nil {
		handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
func (s *Server) HandleAddCarrier(w http.ResponseWriter, r *http.Request) {
	id, ok := pathRequired(w, r, "jobId")
	if !ok {
		return
	}
	var c application.AddCarrierCommand
	if !decode(w, r, &c) {
		return
	}
	c.JobID = id
	v, err := s.app.AddCarrier(r.Context(), c)
	if err != nil {
		handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}
func (s *Server) HandlePreflight(w http.ResponseWriter, r *http.Request) {
	id, ok := pathRequired(w, r, "jobId")
	if !ok {
		return
	}
	type carrierCheck struct {
		CarrierID            string `json:"carrierId"`
		CleaningCompleted    *bool  `json:"cleaningCompleted"`
		AppearancePassed     *bool  `json:"appearancePassed"`
		PlaybackCompatible   *bool  `json:"playbackCompatible"`
		DispositionNote      string `json:"dispositionNote"`
		DispositionCompleted *bool  `json:"dispositionCompleted"`
	}
	var body struct {
		application.Meta
		PlaybackCalibrated *bool          `json:"playbackCalibrated"`
		StorageAvailable   *bool          `json:"storageAvailable"`
		CarrierChecks      []carrierCheck `json:"carrierChecks"`
		CarrierCleaned     bool           `json:"carrierCleaned"`
	}
	if !decode(w, r, &body) {
		return
	}
	c := application.CompletePreflightCommand{Meta: body.Meta, JobID: id, CarrierCleaned: body.CarrierCleaned}
	if body.PlaybackCalibrated == nil || body.StorageAvailable == nil {
		handleError(w, r, domain.Invalid("preflight", "必须提交播放设备校准和存储空间检查结论"))
		return
	}
	c.PlaybackCalibrated = *body.PlaybackCalibrated
	c.StorageAvailable = *body.StorageAvailable
	for i, item := range body.CarrierChecks {
		field := func(name string) string { return "carrierChecks[" + fmt.Sprint(i) + "]." + name }
		if item.CarrierID == "" {
			handleError(w, r, domain.Invalid(field("carrierId"), "不能为空"))
			return
		}
		if item.CleaningCompleted == nil || item.AppearancePassed == nil || item.PlaybackCompatible == nil || item.DispositionCompleted == nil {
			handleError(w, r, domain.Invalid(field("check"), "清洁、外观、播放兼容性和处置完成结论必须完整"))
			return
		}
		c.CarrierChecks = append(c.CarrierChecks, application.CarrierPreflightInput{CarrierID: item.CarrierID, CleaningCompleted: *item.CleaningCompleted, AppearancePassed: *item.AppearancePassed, PlaybackCompatible: *item.PlaybackCompatible, DispositionNote: item.DispositionNote, DispositionCompleted: *item.DispositionCompleted})
	}
	c.JobID = id
	v, err := s.app.CompletePreflight(r.Context(), c)
	if err != nil {
		handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
func (s *Server) HandleCapture(w http.ResponseWriter, r *http.Request) {
	id, ok := pathRequired(w, r, "jobId")
	if !ok {
		return
	}
	var c application.RegisterCaptureCommand
	if !decode(w, r, &c) {
		return
	}
	c.JobID = id
	v, err := s.app.RegisterCapture(r.Context(), c)
	if err != nil {
		handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}
func (s *Server) HandleVoidCapture(w http.ResponseWriter, r *http.Request) {
	job, ok := pathRequired(w, r, "jobId")
	if !ok {
		return
	}
	capture, ok := pathRequired(w, r, "captureId")
	if !ok {
		return
	}
	var c application.VoidCaptureCommand
	if !decode(w, r, &c) {
		return
	}
	c.JobID = job
	c.CaptureID = capture
	v, err := s.app.VoidCapture(r.Context(), c)
	if err != nil {
		handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
func (s *Server) HandleFindings(w http.ResponseWriter, r *http.Request) {
	job, ok := pathRequired(w, r, "jobId")
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		queue, err := s.app.FindingQueue(r.Context(), job, application.FindingQueueFilter{Severity: r.URL.Query().Get("severity"), Source: r.URL.Query().Get("source"), CarrierID: r.URL.Query().Get("carrierId")})
		if err != nil {
			handleError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, queue)
		return
	}
	var c application.AddFindingCommand
	if !decode(w, r, &c) {
		return
	}
	c.JobID = job
	v, err := s.app.AddManualFinding(r.Context(), c)
	if err != nil {
		handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}
func (s *Server) HandleManifestPreview(w http.ResponseWriter, r *http.Request) {
	job, ok := pathRequired(w, r, "jobId")
	if !ok {
		return
	}
	v, err := s.app.ManifestPreview(r.Context(), job)
	if err != nil {
		handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
func (s *Server) HandleReviewFinding(w http.ResponseWriter, r *http.Request) {
	job, ok := pathRequired(w, r, "jobId")
	if !ok {
		return
	}
	finding, ok := pathRequired(w, r, "findingId")
	if !ok {
		return
	}
	var c application.ReviewFindingCommand
	if !decode(w, r, &c) {
		return
	}
	c.JobID = job
	c.FindingID = finding
	v, err := s.app.ReviewFinding(r.Context(), c)
	if err != nil {
		handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
func (s *Server) HandleSubmitApproval(w http.ResponseWriter, r *http.Request) {
	job, ok := pathRequired(w, r, "jobId")
	if !ok {
		return
	}
	var c application.SubmitApprovalCommand
	if !decode(w, r, &c) {
		return
	}
	c.JobID = job
	v, err := s.app.SubmitApproval(r.Context(), c)
	if err != nil {
		handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
func (s *Server) HandleFreezeManifest(w http.ResponseWriter, r *http.Request) {
	job, ok := pathRequired(w, r, "jobId")
	if !ok {
		return
	}
	var c application.FreezeManifestCommand
	if !decode(w, r, &c) {
		return
	}
	c.JobID = job
	v, err := s.app.FreezeManifest(r.Context(), c)
	if err != nil {
		handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}
func (s *Server) HandleIssueCertificate(w http.ResponseWriter, r *http.Request) {
	job, ok := pathRequired(w, r, "jobId")
	if !ok {
		return
	}
	var c application.IssueCertificateCommand
	if !decode(w, r, &c) {
		return
	}
	c.JobID = job
	v, err := s.app.IssueCertificate(r.Context(), c)
	if err != nil {
		handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}
func (s *Server) HandleAudit(w http.ResponseWriter, r *http.Request) {
	job, ok := pathRequired(w, r, "jobId")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"timeline": s.app.AuditTimeline(r.Context(), job)})
}
func (s *Server) HandleVerifyCertificate(w http.ResponseWriter, r *http.Request) {
	number, ok := pathRequired(w, r, "certificateNo")
	if !ok {
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, r, http.StatusBadRequest, "missing_code", "必须提供校验码")
		return
	}
	v := s.app.VerifyCertificate(r.Context(), number, code)
	if !v.Valid {
		writeJSON(w, http.StatusNotFound, v)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
