package httpapi

import (
	"bytes"
	"net/http"
	"time"

	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
	"github.com/Clementine33/preen/backend/internal/workset"
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
	default:
		failed(w, err)
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
		workset.ExecutionRead{Members: true},
	)
	if err != nil {
		writeWorksetError(w, err)
		return false
	}
	return true
}

type coverTargetResponse struct {
	CopyID      string `json:"copy_id"`
	WorkCode    string `json:"rj"`
	LibraryRoot string `json:"library_root"`
	RelPath     string `json:"rel_path"`
	Identity    string `json:"identity"`
}

type coverDestinationResponse struct {
	ID           string              `json:"id"`
	ExecutionID  string              `json:"execution_id"`
	Target       coverTargetResponse `json:"target"`
	Refresh      bool                `json:"refresh"`
	Status       string              `json:"status"`
	ErrorCode    string              `json:"error_code,omitempty"`
	ErrorMessage string              `json:"error_message,omitempty"`
	Published    string              `json:"published,omitempty"`
	Digest       string              `json:"digest,omitempty"`
}

type coverReportResponse struct {
	Items  []coverDestinationResponse `json:"items"`
	Counts map[string]int             `json:"counts"`
}

func toCoverReportResponse(report coverdelivery.Report) coverReportResponse {
	items := make([]coverDestinationResponse, len(report.Items))
	for i, item := range report.Items {
		items[i] = coverDestinationResponse{
			ID:          item.ID,
			ExecutionID: item.ExecutionID,
			Target: coverTargetResponse{
				CopyID:      item.Target.CopyID,
				WorkCode:    item.Target.WorkCode,
				LibraryRoot: item.Target.LibraryRoot,
				RelPath:     item.Target.RelPath,
				Identity:    item.Target.Identity,
			},
			Refresh:      item.Refresh,
			Status:       item.Status,
			ErrorCode:    item.ErrorCode,
			ErrorMessage: item.ErrorMessage,
			Published:    item.Published,
			Digest:       item.Digest,
		}
	}
	return coverReportResponse{Items: items, Counts: report.Counts}
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
	writeJSON(w, http.StatusOK, toCoverReportResponse(report))
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
