package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"tapemastergate/internal/domain"
)

type errorBody struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"requestId"`
		Field     string `json:"field,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	var b errorBody
	b.Error.Code = code
	b.Error.Message = message
	b.Error.RequestID = w.Header().Get("X-Request-ID")
	writeJSON(w, status, b)
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "请求 JSON 无效: "+err.Error())
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "请求只能包含一个 JSON 对象")
		return false
	}
	return true
}
func handleError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusUnprocessableEntity
	code := "validation_error"
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
		code = "not_found"
	case errors.Is(err, domain.ErrConflict):
		status = http.StatusConflict
		code = "version_conflict"
	case errors.Is(err, domain.ErrForbidden):
		status = http.StatusForbidden
		code = "forbidden"
	case errors.Is(err, domain.ErrFrozen):
		status = http.StatusConflict
		code = "job_frozen"
	case errors.Is(err, domain.ErrInvalidTransition):
		status = http.StatusConflict
		code = "invalid_transition"
	case errors.Is(err, domain.ErrOpenSevereFinding):
		status = http.StatusConflict
		code = "open_severe_findings"
	case errors.Is(err, domain.ErrIncompleteCoverage):
		status = http.StatusConflict
		code = "incomplete_coverage"
	case errors.Is(err, domain.ErrDuplicateCapture):
		status = http.StatusConflict
		code = "duplicate_capture_digest"
	case errors.Is(err, domain.ErrFilenameCollision):
		status = http.StatusConflict
		code = "capture_filename_collision"
	case errors.Is(err, domain.ErrLineageConflict):
		status = http.StatusConflict
		code = "capture_lineage_conflict"
	case errors.Is(err, domain.ErrStalePreview):
		status = http.StatusConflict
		code = "stale_manifest_preview"
	case errors.Is(err, domain.ErrManifestBlocked):
		status = http.StatusConflict
		code = "manifest_preview_blocked"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		status = http.StatusConflict
		code = "idempotency_conflict"
	}
	var validation *domain.ValidationError
	if errors.As(err, &validation) {
		var b errorBody
		b.Error.Code = code
		b.Error.Message = err.Error()
		b.Error.RequestID = w.Header().Get("X-Request-ID")
		b.Error.Field = validation.Field
		writeJSON(w, status, b)
		return
	}
	writeError(w, r, status, code, err.Error())
}
func pathRequired(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	v := strings.TrimSpace(r.PathValue(name))
	if v == "" {
		writeError(w, r, http.StatusBadRequest, "missing_path", "路径参数 "+name+" 不能为空")
		return "", false
	}
	return v, true
}
