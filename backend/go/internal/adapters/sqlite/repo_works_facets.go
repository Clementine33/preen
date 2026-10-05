package sqlite

import (
	"github.com/onsei/organizer/backend/internal/works"
)

// facetJoins is, per kind, how a listed work reaches the entity: the circle is
// a column of the work, the voice actors and both tag vocabularies a relation
// table.
var facetJoins = map[works.FacetKind]string{
	works.FacetCircle: `
		JOIN circles e ON e.id = w.circle_id`,
	works.FacetVoiceActor: `
		JOIN work_voice_actors m ON m.work_id = w.id
		JOIN voice_actors e ON e.id = m.voice_actor_id`,
	works.FacetSourceTag: `
		JOIN work_tags m ON m.work_id = w.id
		JOIN tags e ON e.id = m.tag_id`,
	works.FacetUserTag: `
		JOIN work_user_tags m ON m.work_id = w.id
		JOIN user_tags e ON e.id = m.tag_id`,
}

// ListFacets counts, per entity, the listed works that carry it. Only works
// the listing would show count - a relation placeholder never found on disk is
// not a work a filter can return - so an entity with no listed work is absent.
// The filter is applied exactly as the listing applies it, over the same
// joins its fragments name.
func (r *Repository) ListFacets(kind works.FacetKind, filter works.Filter) ([]works.Facet, error) {
	join, ok := facetJoins[kind]
	if !ok {
		return nil, works.Invalid(works.CodeInvalidArgument, "unknown facet")
	}
	builder := listQueryBuilder{}
	builder.applyFilters(filter)
	query := `
		SELECT e.id, e.name, COUNT(DISTINCT w.id)
		FROM works w
		LEFT JOIN work_translations t ON t.work_id = w.id
		LEFT JOIN work_user_data u ON u.work_id = w.id` + join + `
		WHERE w.collected_at IS NOT NULL` + builder.whereTail() + `
		GROUP BY e.id, e.name
		ORDER BY e.name, e.id`
	rows, err := r.db.Query(query, builder.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	facets := []works.Facet{}
	for rows.Next() {
		var facet works.Facet
		if err := rows.Scan(&facet.ID, &facet.Name, &facet.WorkCount); err != nil {
			return nil, err
		}
		facets = append(facets, facet)
	}
	return facets, rows.Err()
}
