package httpapi

import (
	"errors"
	"net/http"

	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/cleanup"
)

func (s *Server) cleanupRoutes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/maintenance/cleanup/preview", protect(http.HandlerFunc(s.previewCleanup)))
	mux.Handle("POST /api/v1/maintenance/cleanup", protect(http.HandlerFunc(s.startCleanup)))
	mux.Handle("GET /api/v1/maintenance/cleanup", protect(http.HandlerFunc(s.readCleanup)))
	mux.Handle("POST /api/v1/maintenance/cleanup/{runId}/cancel", protect(http.HandlerFunc(s.cancelCleanup)))
}
func (s *Server) cleanupService(w http.ResponseWriter) *cleanup.Service {
	if s.deps.Cleanup == nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "cleanup service not configured")
	}
	return s.deps.Cleanup
}
func cleanupCategories(w http.ResponseWriter, r *http.Request) ([]cleanup.Category, error) {
	var req struct {
		Categories []cleanup.Category `json:"categories"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err, "invalid cleanup payload")
		return nil, err
	}
	categories, err := cleanup.Normalize(req.Categories)
	if err != nil {
		writeCleanupError(w, err)
	}
	return categories, err
}
func writeCleanupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cleanup.ErrInvalidCategories):
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, cleanup.ErrNotFound):
		writeError(w, http.StatusNotFound, "CLEANUP_RUN_NOT_FOUND", "cleanup run not found")
	case admission.IsBusy(err):
		writeError(w, http.StatusConflict, "BUSY", "database cleanup cannot start while other activity is running")
	default:
		writeInternal(w, err, "database cleanup failed")
	}
}
func (s *Server) previewCleanup(w http.ResponseWriter, r *http.Request) {
	svc := s.cleanupService(w)
	if svc == nil {
		return
	}
	categories, err := cleanupCategories(w, r)
	if err != nil {
		return
	}
	preview, err := svc.Preview(r.Context(), categories)
	if err != nil {
		writeCleanupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}
func (s *Server) startCleanup(w http.ResponseWriter, r *http.Request) {
	svc := s.cleanupService(w)
	if svc == nil {
		return
	}
	categories, err := cleanupCategories(w, r)
	if err != nil {
		return
	}
	run, err := svc.Start(r.Context(), categories)
	if err != nil {
		writeCleanupError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"run": run})
}
func (s *Server) readCleanup(w http.ResponseWriter, r *http.Request) {
	svc := s.cleanupService(w)
	if svc == nil {
		return
	}
	run, err := svc.Read(r.Context())
	if err != nil {
		writeCleanupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}
func (s *Server) cancelCleanup(w http.ResponseWriter, r *http.Request) {
	svc := s.cleanupService(w)
	if svc == nil {
		return
	}
	var req struct{}
	if err := decodeJSONAllowEmpty(w, r, &req); err != nil {
		writeDecodeError(w, err, "invalid cleanup cancel payload")
		return
	}
	run, err := svc.Cancel(r.Context(), r.PathValue("runId"))
	if err != nil {
		writeCleanupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}
