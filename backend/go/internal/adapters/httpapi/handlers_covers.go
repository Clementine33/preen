package httpapi

import (
	"bytes"
	"net/http"
	"time"

	"github.com/onsei/organizer/backend/internal/works/coverdelivery"
	"github.com/onsei/organizer/backend/internal/workset"
)

func (s *Server) coverService(w http.ResponseWriter) bool {
	if s.deps.Covers == nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "cover delivery is not configured")
		return false
	}
	return true
}
func writeCoverError(w http.ResponseWriter, err error) {
	code := coverdelivery.Code(err)
	status := http.StatusInternalServerError
	switch code {
	case "COPY_NOT_FOUND", "COVER_UNAVAILABLE":
		status = http.StatusNotFound
	case "COVER_CONFLICT", "INPUT_CHANGED":
		status = http.StatusConflict
	case "INVALID_IMAGE":
		status = http.StatusUnprocessableEntity
	}
	writeError(w, status, code, err.Error())
}
func (s *Server) getCopyCover(w http.ResponseWriter, r *http.Request) {
	if !s.coverService(w) {
		return
	}
	code, ok := s.workPath(w, r)
	if !ok {
		return
	}
	image, err := s.deps.Covers.Read(r.Context(), string(code), r.PathValue("copyId"))
	if err != nil {
		writeCoverError(w, err)
		return
	}
	_, mime, err := coverdelivery.Inspect(image.Data)
	if err != nil {
		writeCoverError(w, err)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", `"`+image.Digest+`"`)
	http.ServeContent(w, r, image.Name, time.Time{}, bytes.NewReader(image.Data))
}
func (s *Server) ownedCoverExecution(w http.ResponseWriter, r *http.Request) bool {
	if !s.coverService(w) {
		return false
	}
	svc, err := s.worksetService()
	if err != nil {
		writeError(w, 500, "INTERNAL", "workset service not configured")
		return false
	}
	if r.PathValue("type") != "metadata_refresh" {
		writeError(w, http.StatusNotFound, "EXECUTION_NOT_FOUND", "no metadata execution at this path")
		return false
	}
	_, err = svc.GetExecution(
		r.Context(),
		r.PathValue("id"),
		r.PathValue("type"),
		r.PathValue("executionId"),
		workset.ExecutionPage{Limit: 1},
	)
	if err != nil {
		writeWorksetError(w, err)
		return false
	}
	return true
}
func (s *Server) getExecutionCovers(w http.ResponseWriter, r *http.Request) {
	if !s.ownedCoverExecution(w, r) {
		return
	}
	report, err := s.deps.Covers.Report(r.Context(), r.PathValue("executionId"))
	if err != nil {
		writeCoverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
func (s *Server) retryExecutionCovers(w http.ResponseWriter, r *http.Request) {
	if !s.ownedCoverExecution(w, r) {
		return
	}
	if err := s.deps.Covers.Retry(r.Context(), r.PathValue("executionId")); err != nil {
		writeCoverError(w, err)
		return
	}
	s.getExecutionCovers(w, r)
}
func (s *Server) cancelExecutionCovers(w http.ResponseWriter, r *http.Request) {
	if !s.ownedCoverExecution(w, r) {
		return
	}
	if err := s.deps.Covers.Cancel(r.Context(), r.PathValue("executionId")); err != nil {
		writeCoverError(w, err)
		return
	}
	s.getExecutionCovers(w, r)
}
