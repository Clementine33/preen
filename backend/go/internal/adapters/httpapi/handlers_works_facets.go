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

// listTags answers both tag vocabularies at once: the tag index shows them
// side by side, under the same library scope.
func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	libraries, refusal := facetScope(r)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	out := map[string][]facetResponse{}
	for _, kind := range []struct {
		kind works.FacetKind
		key  string
	}{{works.FacetSourceTag, "source_tags"}, {works.FacetUserTag, "user_tags"}} {
		facets, refusal := service.Facets(kind.kind, libraries)
		if refusal != nil {
			writeWorksError(w, refusal)
			return
		}
		out[kind.key] = toFacetResponses(facets)
	}
	writeJSON(w, http.StatusOK, out)
}

// listFacets answers one facet list under its collection key.
func (s *Server) listFacets(w http.ResponseWriter, r *http.Request, kind works.FacetKind, key string) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	libraries, refusal := facetScope(r)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	facets, refusal := service.Facets(kind, libraries)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]facetResponse{key: toFacetResponses(facets)})
}

// facetScope reads a facet list's only parameter, `library`; anything else is
// refused the way the listing refuses it.
func facetScope(r *http.Request) ([]string, *works.Error) {
	query := r.URL.Query()
	for name := range query {
		if name != "library" {
			return nil, works.Invalid(works.CodeInvalidArgument, "unknown query parameter: "+name)
		}
	}
	var libraries []string
	for part := range strings.SplitSeq(query.Get("library"), ",") {
		if part = strings.TrimSpace(part); part != "" {
			libraries = append(libraries, part)
		}
	}
	return libraries, nil
}

func toFacetResponses(facets []works.Facet) []facetResponse {
	out := make([]facetResponse, 0, len(facets))
	for _, facet := range facets {
		out = append(out, facetResponse{ID: facet.ID, Name: facet.Name, WorkCount: facet.WorkCount})
	}
	return out
}
