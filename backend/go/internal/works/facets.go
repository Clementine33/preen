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
// ordered by name. Naming libraries scopes both the entities and their counts
// to works with a copy there, the way the listing's library filter does.
func (s *Service) Facets(kind FacetKind, libraries []string) ([]Facet, *Error) {
	facets, err := s.store.ListFacets(kind, libraries)
	if err != nil {
		return nil, asInternal(err)
	}
	return facets, nil
}
