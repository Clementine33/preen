package httpapi

import (
	"net/http"

	"github.com/onsei/organizer/backend/internal/collections"
	"github.com/onsei/organizer/backend/internal/works"
)

func (s *Server) collectionService(w http.ResponseWriter) (*collections.Service, bool) {
	if s.deps.Collections == nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "collection service is not configured")
		return nil, false
	}
	return s.deps.Collections, true
}

// collectionWindow reads the paging keys; details also accept `direction` and `sort`.
func collectionWindow(r *http.Request, withDirection bool) (collections.Window, *works.Error) {
	for key := range r.URL.Query() {
		if key != "limit" && key != "page" && (!withDirection || key != "direction" && key != "sort") {
			return collections.Window{}, works.Invalid(works.CodeInvalidArgument, "unknown query parameter: "+key)
		}
	}
	limit, invalid := parseLimit(r)
	if invalid != nil {
		return collections.Window{}, invalid
	}
	page, invalid := parsePage(r)
	direction := works.Direction(r.URL.Query().Get("direction"))
	return collections.Window{Limit: limit, Page: page, Direction: direction,
		Sort: works.SortField(r.URL.Query().Get("sort"))}, invalid
}

func (s *Server) listCollections(w http.ResponseWriter, r *http.Request) {
	service, ok := s.collectionService(w)
	if !ok {
		return
	}
	window, invalid := collectionWindow(r, false)
	if invalid != nil {
		writeWorksError(w, invalid)
		return
	}
	page, err := service.List(r.Context(), window)
	if err != nil {
		writeWorksError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) createCollection(w http.ResponseWriter, r *http.Request) {
	service, ok := s.collectionService(w)
	if !ok {
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeDecodeError(w, err, "invalid collection name")
		return
	}
	c, err := service.Create(r.Context(), request.Name)
	if err != nil {
		writeWorksError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) getCollection(w http.ResponseWriter, r *http.Request) {
	service, ok := s.collectionService(w)
	if !ok {
		return
	}
	window, invalid := collectionWindow(r, true)
	if invalid != nil {
		writeWorksError(w, invalid)
		return
	}
	detail, err := service.Detail(r.Context(), r.PathValue("id"), window)
	if err != nil {
		writeWorksError(w, err)
		return
	}
	out := struct {
		collections.Collection

		Works []workSummaryResponse `json:"works"`
		Total int                   `json:"total"`
	}{Collection: detail.Collection, Works: make([]workSummaryResponse, len(detail.Works)), Total: detail.Total}
	rows := make([]*workSummaryResponse, len(detail.Works))
	for i, summary := range detail.Works {
		out.Works[i] = toWorkSummary(summary)
		rows[i] = &out.Works[i]
	}
	s.attachLocalCovers(r, rows...)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) appendCollectionWorks(w http.ResponseWriter, r *http.Request) {
	service, ok := s.collectionService(w)
	if !ok {
		return
	}
	var request struct {
		RJs []string `json:"rjs"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeDecodeError(w, err, "invalid collection members")
		return
	}
	result, err := service.Append(r.Context(), r.PathValue("id"), request.RJs)
	if err != nil {
		writeWorksError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) renameCollection(w http.ResponseWriter, r *http.Request) {
	service, ok := s.collectionService(w)
	if !ok {
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeDecodeError(w, err, "invalid collection name")
		return
	}
	c, err := service.Rename(r.Context(), r.PathValue("id"), request.Name)
	if err != nil {
		writeWorksError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) deleteCollection(w http.ResponseWriter, r *http.Request) {
	service, ok := s.collectionService(w)
	if !ok {
		return
	}
	if err := service.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeWorksError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) collectionRoutes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/collections", protect(http.HandlerFunc(s.listCollections)))
	mux.Handle("POST /api/v1/collections", protect(http.HandlerFunc(s.createCollection)))
	mux.Handle("GET /api/v1/collections/{id}", protect(http.HandlerFunc(s.getCollection)))
	mux.Handle("PATCH /api/v1/collections/{id}", protect(http.HandlerFunc(s.renameCollection)))
	mux.Handle("DELETE /api/v1/collections/{id}", protect(http.HandlerFunc(s.deleteCollection)))
	mux.Handle("POST /api/v1/collections/{id}/works", protect(http.HandlerFunc(s.appendCollectionWorks)))
	mux.Handle("DELETE /api/v1/collections/{id}/works/{rj}", protect(http.HandlerFunc(s.removeCollectionWork)))
	mux.Handle("PATCH /api/v1/collections/{id}/works/{rj}", protect(http.HandlerFunc(s.moveCollectionWork)))
}
