package httpapi

import (
	"net/http"

	"github.com/Clementine33/preen/backend/internal/works"
)

// facetResponse is one filterable entity: the id the listing parameter of the
// same name takes, and how many listed works carry it.
type facetResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	WorkCount int    `json:"work_count"`
}

// circleFacetsResponse is the GET /api/v1/circles payload.
type circleFacetsResponse struct {
	Circles []facetResponse `json:"circles"`
}

// voiceActorFacetsResponse is the GET /api/v1/voice-actors payload.
type voiceActorFacetsResponse struct {
	VoiceActors []facetResponse `json:"voice_actors"`
}

// tagFacetsResponse is the GET /api/v1/tags payload.
type tagFacetsResponse struct {
	SourceTags []facetResponse `json:"source_tags"`
	UserTags   []facetResponse `json:"user_tags"`
}

// listCircles answers the circles the listing can filter by.
func (s *Server) listCircles(w http.ResponseWriter, r *http.Request) {
	facets, ok := s.loadFacets(w, r, works.FacetCircle)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, circleFacetsResponse{Circles: facets})
}

// listVoiceActors answers the voice actors the listing can filter by.
func (s *Server) listVoiceActors(w http.ResponseWriter, r *http.Request) {
	facets, ok := s.loadFacets(w, r, works.FacetVoiceActor)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, voiceActorFacetsResponse{VoiceActors: facets})
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
	sourceTags, refusal := service.Facets(works.FacetSourceTag, filter)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	userTags, refusal := service.Facets(works.FacetUserTag, filter)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, tagFacetsResponse{
		SourceTags: toFacetResponses(sourceTags),
		UserTags:   toFacetResponses(userTags),
	})
}

// loadFacets reads one facet list under the current filter.
func (s *Server) loadFacets(w http.ResponseWriter, r *http.Request, kind works.FacetKind) ([]facetResponse, bool) {
	service, ok := s.catalog(w)
	if !ok {
		return nil, false
	}
	filter, refusal := facetFilter(r)
	if refusal != nil {
		writeWorksError(w, refusal)
		return nil, false
	}
	facets, refusal := service.Facets(kind, filter)
	if refusal != nil {
		writeWorksError(w, refusal)
		return nil, false
	}
	return toFacetResponses(facets), true
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
