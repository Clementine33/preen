package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// startExecution handles POST
// /api/v1/worksets/{id}/operations/{type}/revisions/{planId}/executions.
// If-Match (operation version) and Idempotency-Key are both required. The body
// is optional and carries one thing: `folder_paths` scopes the session to those
// members of the record, and an absent or empty list runs the whole revision.
// The worklist and the session options stay the frozen revision's.
func (s *Server) startExecution(w http.ResponseWriter, r *http.Request) {
	svc, err := s.worksetService()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "workset service not configured")
		return
	}
	idemKey := r.Header.Get("Idempotency-Key")
	if idemKey == "" {
		writeError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required")
		return
	}
	version, valid := ifMatchVersion(r)
	if !valid {
		writeError(
			w,
			http.StatusBadRequest,
			"VERSION_REQUIRED",
			"If-Match header with the operation version is required",
		)
		return
	}
	var body struct {
		FolderPaths []string `json:"folder_paths"`
	}
	if decodeErr := decodeJSONAllowEmpty(w, r, &body); decodeErr != nil {
		writeDecodeError(w, decodeErr, "invalid execution request body")
		return
	}
	res, err := svc.StartExecution(
		r.Context(),
		r.PathValue("id"),
		r.PathValue("type"),
		r.PathValue("planId"),
		workset.StartExecutionRequest{
			IfMatchVersion: version,
			IdempotencyKey: idemKey,
			FolderPaths:    body.FolderPaths,
		},
	)
	if err != nil {
		writeWorksetError(w, err)
		return
	}
	status := http.StatusAccepted
	if !res.Created {
		status = http.StatusOK
	}
	writeJSON(w, status, struct {
		Created   bool                  `json:"created"`
		Execution workset.ExecutionView `json:"execution"`
	}{Created: res.Created, Execution: *res.Execution})
}

// getExecution handles GET
// /api/v1/worksets/{id}/operations/{type}/executions/{executionId}. Without
// parameters the response carries every component. `view=members` swaps the
// components for per-member summaries; `member_id=<id>` narrows the components
// to one member and adds that member's summary. The two do not combine.
func (s *Server) getExecution(w http.ResponseWriter, r *http.Request) {
	svc, err := s.worksetService()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "workset service not configured")
		return
	}
	read, err := executionReadOf(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_VIEW", err.Error())
		return
	}
	view, err := svc.GetExecution(
		r.Context(),
		r.PathValue("id"),
		r.PathValue("type"),
		r.PathValue("executionId"),
		read,
	)
	if err != nil {
		writeWorksetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// executionReadOf reads the optional shape parameters of a detail request.
func executionReadOf(r *http.Request) (workset.ExecutionRead, error) {
	query := r.URL.Query()
	view, memberID := query.Get("view"), query.Get("member_id")
	switch {
	case view != "" && memberID != "":
		return workset.ExecutionRead{}, errors.New("view and member_id cannot be combined")
	case view != "" && view != "members":
		return workset.ExecutionRead{}, errors.New("view must be members")
	}
	return workset.ExecutionRead{Members: view == "members", MemberID: memberID}, nil
}

// cancelExecution handles POST
// /api/v1/worksets/{id}/operations/{type}/executions/{executionId}/cancel.
func (s *Server) cancelExecution(w http.ResponseWriter, r *http.Request) {
	svc, err := s.worksetService()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "workset service not configured")
		return
	}
	view, err := svc.CancelExecution(r.Context(), r.PathValue("id"), r.PathValue("type"), r.PathValue("executionId"))
	if err != nil {
		writeWorksetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// executionEvents handles GET
// /api/v1/worksets/{id}/operations/{type}/executions/{executionId}/events
// (SSE): an authoritative snapshot first, then progress and a terminal event.
// `view=members` streams the members summary and `member` events instead of
// components and `component` events.
func (s *Server) executionEvents(w http.ResponseWriter, r *http.Request) {
	svc, err := s.worksetService()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "workset service not configured")
		return
	}
	subscribe := svc.SubscribeExecution
	if view := r.URL.Query().Get("view"); view != "" {
		if view != "members" {
			writeError(w, http.StatusBadRequest, "INVALID_VIEW", "view must be members")
			return
		}
		subscribe = svc.SubscribeExecutionMembers
	}
	streamSessionEvents(w, r, "EXECUTION_NOT_FOUND", func(ctx context.Context, emit func(string, any) error) error {
		return subscribe(ctx, r.PathValue("id"), r.PathValue("type"), r.PathValue("executionId"), emit)
	})
}
