package works

// FacetKind is a source entity the listing can filter by and a caller can list
// to offer that filter: the circle, the voice actor, the source's tags and the
// user's own.
type FacetKind string

// The listable facets. Each one names the listing parameter it feeds; the two
// tag kinds also feed the combined `tag` parameter.
const (
	FacetCircle     FacetKind = "circle"
	FacetVoiceActor FacetKind = "voice_actor"
	FacetSourceTag  FacetKind = "source_tag"
	FacetUserTag    FacetKind = "user_tag"
)

// Facet is one filterable entity: the id the listing parameter takes, the name
// to show, and how many listed works carry it under the same library scope.
type Facet struct {
	ID        string
	Name      string
	WorkCount int
}

// Facets lists the entities of one kind that at least one listed work carries,
// ordered by name. The filter is the listing's own, so the entities and their
// counts narrow with the wall: a caller who wants the alternatives to its own
// pick leaves that dimension out.
func (s *Service) Facets(kind FacetKind, filter Filter) ([]Facet, *Error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	facets, err := s.store.ListFacets(kind, filter)
	if err != nil {
		return nil, asInternal(err)
	}
	return facets, nil
}
