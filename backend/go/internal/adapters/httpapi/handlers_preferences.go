package httpapi

import (
	"net/http"

	"github.com/Clementine33/preen/backend/internal/works"
)

// preferencesResponse is the GET/PUT /api/v1/preferences payload.
type preferencesResponse struct {
	TrackFormats     []string `json:"track_formats"`
	DefaultLibraryID *string  `json:"default_library_id,omitempty"`
}

// preferencesUpdateRequest is the PUT /api/v1/preferences merge payload.
type preferencesUpdateRequest struct {
	TrackFormats     *[]string      `json:"track_formats"`
	DefaultLibraryID nullableString `json:"default_library_id"`
}

func toPreferencesResponse(prefs works.Preferences) preferencesResponse {
	return preferencesResponse{
		TrackFormats:     prefs.TrackFormats,
		DefaultLibraryID: prefs.DefaultLibraryID,
	}
}

// getPreferences handles GET /api/v1/preferences.
func (s *Server) getPreferences(w http.ResponseWriter, _ *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	prefs, refusal := service.Preferences()
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, toPreferencesResponse(prefs))
}

// putPreferences handles PUT /api/v1/preferences: a merge, last write wins per field.
func (s *Server) putPreferences(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	var req preferencesUpdateRequest
	if decodeErr := decodeJSON(w, r, &req); decodeErr != nil {
		writeDecodeError(w, decodeErr, "invalid preferences payload")
		return
	}
	prefs, refusal := service.SavePreferences(works.PreferencesUpdate{
		TrackFormats:     req.TrackFormats,
		DefaultLibraryID: req.DefaultLibraryID.toWorks(),
	})
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, toPreferencesResponse(prefs))
}
