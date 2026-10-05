package httpapi

import (
	"net/http"

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
// side by side, under the same filter.
func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	filter, refusal := facetFilter(r)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	out := map[string][]facetResponse{}
	for _, kind := range []struct {
		kind works.FacetKind
		key  string
	}{{works.FacetSourceTag, "source_tags"}, {works.FacetUserTag, "user_tags"}} {
		facets, refusal := service.Facets(kind.kind, filter)
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
	filter, refusal := facetFilter(r)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	facets, refusal := service.Facets(kind, filter)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]facetResponse{key: toFacetResponses(facets)})
}

// facetFilter reads a facet list's parameters: the listing's filters, without
// its order, page size or page number, which describe a page rather than the
// set of works. Anything else is refused the way the listing refuses it.
func facetFilter(r *http.Request) (works.Filter, *works.Error) {
	for name := range r.URL.Query() {
		switch name {
		case "sort", "direction", "limit", "page":
			return works.Filter{}, works.Invalid(works.CodeInvalidArgument, "unknown query parameter: "+name)
		}
	}
	return parseWorkFilter(r)
}

func toFacetResponses(facets []works.Facet) []facetResponse {
	out := make([]facetResponse, 0, len(facets))
	for _, facet := range facets {
		out = append(out, facetResponse{ID: facet.ID, Name: facet.Name, WorkCount: facet.WorkCount})
	}
	return out
}
