package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// taskSettingsResponse is the deployment-wide common settings of one Task kind.
// The task owns the payload; the ETag header carries the concurrency token.
type taskSettingsResponse struct {
	OperationType string          `json:"operation_type"`
	Settings      json.RawMessage `json:"settings"`
}

// taskSettingsRequest is the PUT /api/v1/tasks/{type}/settings payload.
type taskSettingsRequest struct {
	Settings json.RawMessage `json:"settings"`
}

// taskSettingsRoutes registers the settings of a Task kind: independent of any
// record, and the only write path of its common settings (ADR 0008).
func (s *Server) taskSettingsRoutes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/tasks/{type}/settings", protect(http.HandlerFunc(s.getTaskSettings)))
	mux.Handle("PUT /api/v1/tasks/{type}/settings", protect(http.HandlerFunc(s.putTaskSettings)))
}

func writeTaskSettings(w http.ResponseWriter, s *workset.TaskSettings) {
	w.Header().Set("ETag", `"`+s.ETag+`"`)
	writeJSON(w, http.StatusOK, taskSettingsResponse{OperationType: s.OperationType, Settings: s.Settings})
}

// getTaskSettings handles GET /api/v1/tasks/{type}/settings.
func (s *Server) getTaskSettings(w http.ResponseWriter, r *http.Request) {
	svc, err := s.worksetService()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "workset service not configured")
		return
	}
	settings, err := svc.GetTaskSettings(r.Context(), r.PathValue("type"))
	if err != nil {
		writeWorksetError(w, err)
		return
	}
	writeTaskSettings(w, settings)
}

// putTaskSettings handles PUT /api/v1/tasks/{type}/settings: the only write
// path of a Task kind's common settings, If-Match on the ETag read.
func (s *Server) putTaskSettings(w http.ResponseWriter, r *http.Request) {
	svc, err := s.worksetService()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "workset service not configured")
		return
	}
	var req taskSettingsRequest
	if decodeErr := decodeJSON(w, r, &req); decodeErr != nil {
		writeDecodeError(w, decodeErr, "invalid settings payload")
		return
	}
	if len(req.Settings) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "settings is required")
		return
	}
	etag := strings.Trim(strings.TrimPrefix(r.Header.Get("If-Match"), "W/"), `"`)
	if etag == "" {
		writeError(w, http.StatusBadRequest, "VERSION_REQUIRED", "If-Match header with the settings ETag is required")
		return
	}
	settings, err := svc.SaveTaskSettings(r.Context(), r.PathValue("type"), req.Settings, etag)
	if err != nil {
		writeWorksetError(w, err)
		return
	}
	writeTaskSettings(w, settings)
}
