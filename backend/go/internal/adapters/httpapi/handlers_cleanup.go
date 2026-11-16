package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/cleanup"
)

type cleanupCategory string

type cleanupCategoriesRequest struct {
	Categories []cleanupCategory `json:"categories"`
}

type cleanupRuleResponse struct {
	Protection string `json:"protection"`
}

type cleanupPreviewResponse struct {
	CheckedAt  time.Time                               `json:"checked_at"`
	Categories []cleanupCategory                       `json:"categories"`
	Candidates map[cleanupCategory]int64               `json:"candidates"`
	Rules      map[cleanupCategory]cleanupRuleResponse `json:"rules"`
}

type cleanupCountResponse struct {
	Checked int64 `json:"checked"`
	Deleted int64 `json:"deleted"`
	Skipped int64 `json:"skipped"`
}

type cleanupRunResponse struct {
	ID         string                                   `json:"run_id"`
	Categories []cleanupCategory                        `json:"categories"`
	Status     string                                   `json:"status"`
	Phase      cleanupCategory                          `json:"phase"`
	StartedAt  time.Time                                `json:"started_at"`
	FinishedAt *time.Time                               `json:"finished_at"`
	Counts     map[cleanupCategory]cleanupCountResponse `json:"counts"`
	Reason     string                                   `json:"reason"`
}

type cleanupRunEnvelopeResponse struct {
	Run *cleanupRunResponse `json:"run"`
}

func toCleanupCategories(in []cleanup.Category) []cleanupCategory {
	out := make([]cleanupCategory, len(in))
	for i, c := range in {
		out[i] = cleanupCategory(c)
	}
	return out
}

func toCleanupPreviewResponse(p cleanup.Preview) cleanupPreviewResponse {
	candidates := make(map[cleanupCategory]int64, len(p.Candidates))
	for k, v := range p.Candidates {
		candidates[cleanupCategory(k)] = v
	}
	rules := make(map[cleanupCategory]cleanupRuleResponse, len(p.Rules))
	for k, v := range p.Rules {
		rules[cleanupCategory(k)] = cleanupRuleResponse{Protection: v.Protection}
	}
	return cleanupPreviewResponse{
		CheckedAt:  p.CheckedAt,
		Categories: toCleanupCategories(p.Categories),
		Candidates: candidates,
		Rules:      rules,
	}
}

func toCleanupRunResponse(r *cleanup.Run) *cleanupRunResponse {
	if r == nil {
		return nil
	}
	var counts map[cleanupCategory]cleanupCountResponse
	if r.Counts != nil {
		counts = make(map[cleanupCategory]cleanupCountResponse, len(r.Counts))
		for k, v := range r.Counts {
			counts[cleanupCategory(k)] = cleanupCountResponse{
				Checked: v.Checked,
				Deleted: v.Deleted,
				Skipped: v.Skipped,
			}
		}
	}
	return &cleanupRunResponse{
		ID:         r.ID,
		Categories: toCleanupCategories(r.Categories),
		Status:     r.Status,
		Phase:      cleanupCategory(r.Phase),
		StartedAt:  r.StartedAt,
		FinishedAt: r.FinishedAt,
		Counts:     counts,
		Reason:     r.Reason,
	}
}

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
	var req cleanupCategoriesRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err, "invalid cleanup payload")
		return nil, err
	}
	raw := make([]cleanup.Category, len(req.Categories))
	for i, c := range req.Categories {
		raw[i] = cleanup.Category(c)
	}
	categories, err := cleanup.Normalize(raw)
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
	writeJSON(w, http.StatusOK, toCleanupPreviewResponse(preview))
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
	writeJSON(w, http.StatusAccepted, cleanupRunEnvelopeResponse{Run: toCleanupRunResponse(run)})
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
	writeJSON(w, http.StatusOK, cleanupRunEnvelopeResponse{Run: toCleanupRunResponse(run)})
}
func (s *Server) cancelCleanup(w http.ResponseWriter, r *http.Request) {
	svc := s.cleanupService(w)
	if svc == nil {
		return
	}
	var req emptyRequest
	if err := decodeJSONAllowEmpty(w, r, &req); err != nil {
		writeDecodeError(w, err, "invalid cleanup cancel payload")
		return
	}
	run, err := svc.Cancel(r.Context(), r.PathValue("runId"))
	if err != nil {
		writeCleanupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cleanupRunEnvelopeResponse{Run: toCleanupRunResponse(run)})
}
