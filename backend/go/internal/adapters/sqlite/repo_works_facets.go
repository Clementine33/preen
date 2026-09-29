package sqlite

import (
	"github.com/onsei/organizer/backend/internal/works"
)

// facetJoins is, per kind, how a listed work reaches the entity: the circle is
// a column of the work, the voice actors a relation table.
var facetJoins = map[works.FacetKind]string{
	works.FacetCircle: `
		JOIN circles e ON e.id = w.circle_id`,
	works.FacetVoiceActor: `
		JOIN work_voice_actors m ON m.work_id = w.id
		JOIN voice_actors e ON e.id = m.voice_actor_id`,
}

// ListFacets counts, per entity, the listed works that carry it. Only works
// the listing would show count - a relation placeholder never found on disk is
// not a work a filter can return - so an entity with no listed work is absent.
func (r *Repository) ListFacets(kind works.FacetKind, libraries []string) ([]works.Facet, error) {
	join, ok := facetJoins[kind]
	if !ok {
		return nil, works.Invalid(works.CodeInvalidArgument, "unknown facet")
	}
	query := `
		SELECT e.id, e.name, COUNT(DISTINCT w.id)
		FROM works w` + join + `
		WHERE w.collected_at IS NOT NULL`
	args := []any{}
	if len(libraries) > 0 {
		query += `
		  AND EXISTS (SELECT 1 FROM work_copies wc
		              WHERE wc.work_id = w.id AND wc.library_id IN (` + placeholderList(len(libraries)) + `))`
		args = textArgs(libraries)
	}
	query += `
		GROUP BY e.id, e.name
		ORDER BY e.name, e.id`
	rows, err := r.db.Query(query, args...)
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
