package httpapi

import (
	"net/http"

	"github.com/Clementine33/preen/backend/internal/works"
)

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
	writeJSON(w, http.StatusOK, prefs)
}

// putPreferences handles PUT /api/v1/preferences: last write wins.
func (s *Server) putPreferences(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	var req works.Preferences
	if decodeErr := decodeJSON(w, r, &req); decodeErr != nil {
		writeDecodeError(w, decodeErr, "invalid preferences payload")
		return
	}
	prefs, refusal := service.SavePreferences(req)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, prefs)
}
