package httpapi

import (
	"encoding/json"
	"net/http"
)

func (s *Server) removeCollectionWork(w http.ResponseWriter, r *http.Request) {
	service, ok := s.collectionService(w)
	if !ok {
		return
	}
	work, ok := s.workPath(w, r)
	if !ok {
		return
	}
	if err := service.Remove(r.Context(), r.PathValue("id"), work); err != nil {
		writeWorksError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) moveCollectionWork(w http.ResponseWriter, r *http.Request) {
	service, ok := s.collectionService(w)
	if !ok {
		return
	}
	work, ok := s.workPath(w, r)
	if !ok {
		return
	}
	var request struct {
		Before json.RawMessage `json:"before_rj"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeDecodeError(w, err, "invalid collection move")
		return
	}
	var before *string
	if len(request.Before) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "before_rj is required; null moves to the end")
		return
	}
	if err := json.Unmarshal(request.Before, &before); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "before_rj must be a work number or null")
		return
	}
	if err := service.Move(r.Context(), r.PathValue("id"), work, before); err != nil {
		writeWorksError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
