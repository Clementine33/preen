package httpapi

import (
	"net/http"

	"github.com/Clementine33/preen/backend/internal/works"
)

// listeningRoutes registers the Listening Position routes of a work.
func (s *Server) listeningRoutes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("PUT /api/v1/works/{rj}/listening", protect(http.HandlerFunc(s.putWorkListening)))
	mux.Handle("DELETE /api/v1/works/{rj}/listening", protect(http.HandlerFunc(s.deleteWorkListening)))
}

// listeningResponse is one Rendition's stored Listening Position.
type listeningResponse struct {
	Rendition   string   `json:"rendition"`
	TrackKey    *string  `json:"track_key"`
	PositionSec *float64 `json:"position_sec"`
	TrackIndex  *int     `json:"track_index"`
	TrackCount  *int     `json:"track_count"`
	FinishedAt  *string  `json:"finished_at"`
	UpdatedAt   string   `json:"updated_at"`
}

func toListening(row works.Listening) listeningResponse {
	return listeningResponse(row)
}

// putWorkListening stores one Listening Position snapshot and answers the
// stored row.
func (s *Server) putWorkListening(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	code, ok := s.workPath(w, r)
	if !ok {
		return
	}
	var req works.ListeningRequest
	if decodeErr := decodeJSON(w, r, &req); decodeErr != nil {
		writeDecodeError(w, decodeErr, "invalid listening payload")
		return
	}
	row, refusal := service.PutListening(code, req)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, toListening(row))
}

// deleteWorkListening forgets every Listening Position and Finished mark.
func (s *Server) deleteWorkListening(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	code, ok := s.workPath(w, r)
	if !ok {
		return
	}
	if refusal := service.ClearListening(code); refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
