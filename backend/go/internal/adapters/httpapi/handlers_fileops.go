package httpapi

import (
	"errors"
	"net/http"

	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/fileops"
)

// fileOperationRequest is the POST /api/v1/libraries/{id}/file-operations
// payload. The member is addressed by its library-relative path; the items are
// addressed relative to that member.
type fileOperationRequest struct {
	MemberPath string              `json:"member_path"`
	Operation  string              `json:"operation"`
	Items      []fileOperationItem `json:"items"`
}

// fileOperationItem is one requested change. Name belongs to a rename (a plain
// name, never a path), TargetDir to a move.
type fileOperationItem struct {
	Source    string `json:"source"`
	Name      string `json:"name,omitempty"`
	TargetDir string `json:"target_dir,omitempty"`
}

// fileOperationItemResponse is what happened to one requested item.
type fileOperationItemResponse struct {
	Source        string `json:"source"`
	Status        string `json:"status"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
	Target        string `json:"target,omitempty"`
	RecoveredPath string `json:"recovered_path,omitempty"`
}

// fileOperationRefreshResponse reports the post-write inventory refresh state.
type fileOperationRefreshResponse struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// fileOperationResponse is the POST /api/v1/libraries/{id}/file-operations payload.
type fileOperationResponse struct {
	Operation  string                       `json:"operation"`
	MemberPath string                       `json:"member_path"`
	Items      []fileOperationItemResponse  `json:"items"`
	Succeeded  int                          `json:"succeeded"`
	Failed     int                          `json:"failed"`
	Untouched  int                          `json:"untouched"`
	Refresh    fileOperationRefreshResponse `json:"refresh"`
}

func toFileOperationResponse(res *fileops.Result) fileOperationResponse {
	items := make([]fileOperationItemResponse, len(res.Items))
	for i, item := range res.Items {
		items[i] = fileOperationItemResponse{
			Source:        item.Source,
			Status:        item.Status,
			Code:          item.Code,
			Message:       item.Message,
			Target:        item.Target,
			RecoveredPath: item.RecoveredPath,
		}
	}
	return fileOperationResponse{
		Operation:  res.Operation,
		MemberPath: res.MemberPath,
		Items:      items,
		Succeeded:  res.Succeeded,
		Failed:     res.Failed,
		Untouched:  res.Untouched,
		Refresh: fileOperationRefreshResponse{
			OK:      res.Refresh.OK,
			Code:    res.Refresh.Code,
			Message: res.Refresh.Message,
		},
	}
}

// applyFileOperation performs direct file management inside one member
// directory: single-item rename, single-item move inside the member, or a
// batch soft delete into the library's recovery directory.
//
// The whole request holds the direct-file-management slot, so it is refused
// while any scan, planning session or execution is running, and it refuses
// those in turn. Direct file management is its own write path with its own
// path, conflict and admission rules (ADR 0002); it is not a plan and
// is not recorded as one.
func (s *Server) applyFileOperation(w http.ResponseWriter, r *http.Request) {
	lib, ok := s.library(w, r)
	if !ok {
		return
	}
	if s.deps.FileOps == nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "file operations are not configured")
		return
	}
	var req fileOperationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err, "invalid file operation payload")
		return
	}
	items := make([]fileops.Item, 0, len(req.Items))
	for _, item := range req.Items {
		items = append(items, fileops.Item{Source: item.Source, Name: item.Name, TargetDir: item.TargetDir})
	}

	result, err := s.deps.FileOps.Apply(r.Context(), fileops.Request{
		LibraryRoot: lib.RootPath,
		MemberPath:  req.MemberPath,
		Operation:   req.Operation,
		Items:       items,
	})
	if err != nil {
		if admission.IsBusy(err) {
			writeBusyError(w, err)
			return
		}
		// A refusal of the whole request (an unsupported operation, an
		// unusable member, a malformed path, no items): the member never
		// became a scope, so nothing ran and nothing was written.
		if itemErr, ok := errors.AsType[*fileops.PathError](err); ok {
			writeError(w, http.StatusBadRequest, itemErr.Code, itemErr.Message)
			return
		}
		writeError(w, http.StatusBadRequest, "REQUEST_REFUSED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toFileOperationResponse(result))
}
