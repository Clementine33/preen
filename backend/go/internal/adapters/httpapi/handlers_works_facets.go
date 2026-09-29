package httpapi

import (
	"net/http"
	"strings"

	"github.com/onsei/organizer/backend/internal/works"
)

// facetResponse is one filterable entity: the id the listing parameter of the
// same name takes, and how many listed works carry it.
type facetResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	WorkCount int    `json:"work_count"`
}

// listCircles answers the circles the listing can filter by.
func (s *Server) listCircles(w http.ResponseWriter, r *http.Request) {
	s.listFacets(w, r, works.FacetCircle, "circles")
}

// listVoiceActors answers the voice actors the listing can filter by.
func (s *Server) listVoiceActors(w http.ResponseWriter, r *http.Request) {
	s.listFacets(w, r, works.FacetVoiceActor, "voice_actors")
}

// listFacets answers one facet list under its collection key. Only `library`
// is accepted, and anything else is refused the way the listing refuses it.
func (s *Server) listFacets(w http.ResponseWriter, r *http.Request, kind works.FacetKind, key string) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	query := r.URL.Query()
	for name := range query {
		if name != "library" {
			writeWorksError(w, works.Invalid(works.CodeInvalidArgument, "unknown query parameter: "+name))
			return
		}
	}
	var libraries []string
	for part := range strings.SplitSeq(query.Get("library"), ",") {
		if part = strings.TrimSpace(part); part != "" {
			libraries = append(libraries, part)
		}
	}
	facets, refusal := service.Facets(kind, libraries)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	out := make([]facetResponse, 0, len(facets))
	for _, facet := range facets {
		out = append(out, facetResponse{ID: facet.ID, Name: facet.Name, WorkCount: facet.WorkCount})
	}
	writeJSON(w, http.StatusOK, map[string][]facetResponse{key: out})
}
