package httpapi

import (
	"net/http"

	"github.com/Clementine33/preen/backend/internal/updatecheck"
)

// updateCheckResponse is the wire form of the update check; the release fields
// appear only while a newer release is available (docs/api/update-check.md).
type updateCheckResponse struct {
	State         string `json:"state"`
	LatestVersion string `json:"latest_version,omitempty"`
	ReleaseURL    string `json:"release_url,omitempty"`
}

// getUpdateCheck handles GET /api/v1/update-check. A server wired without the
// check reports none, like one whose check was skipped.
func (s *Server) getUpdateCheck(w http.ResponseWriter, _ *http.Request) {
	status := updatecheck.Status{State: updatecheck.StateNone}
	if s.deps.UpdateCheck != nil {
		status = s.deps.UpdateCheck.Status()
	}
	resp := updateCheckResponse{State: status.State}
	if status.State == updatecheck.StateAvailable {
		resp.LatestVersion, resp.ReleaseURL = status.LatestVersion, status.ReleaseURL
	}
	writeJSON(w, http.StatusOK, resp)
}
