package works

import "slices"

// Relation violations: a structural refusal refuses the whole submission and
// writes nothing. Each is a code the HTTP layer maps to a 400.
const (
	CodeRelationSelfReference = "RELATION_SELF_REFERENCE"
	CodeRelationOriginalChain = "RELATION_ORIGINAL_CHAIN"
	CodeRelationCycle         = "RELATION_CYCLE"
)

// RelationViolation is one structural refusal the relation rules found.
type RelationViolation struct {
	Code    string
	WorkID  WorkCode
	Message string
}

// ValidateRelations refuses the structural shapes the catalog may not hold:
// a work that is its own original or parent, an original chain, and a cycle in
// the effective parent edges.
//
// Only the effective parent edges are checked for a cycle. A declaration that
// a work's own report outranks stays in the evidence tables and may still
// point back at the work that outranked it - that is stored disagreement, not
// a relation, and it does not participate in inheritance.
func ValidateRelations(ev Evidence) *RelationViolation {
	return ValidateResolvedRelations(ev, Resolve(ev))
}

// ValidateResolvedRelations checks evidence against its already computed
// resolution so transactional import can reuse it for validation and storage.
func ValidateResolvedRelations(ev Evidence, resolution Resolution) *RelationViolation {
	for _, id := range ev.sortedSelfIDs() {
		self := ev.Self[id]
		switch {
		case self.ParentRJ != nil && *self.ParentRJ == id:
			return &RelationViolation{
				Code:    CodeRelationSelfReference,
				WorkID:  id,
				Message: "a work cannot be its own parent",
			}
		case self.OriginalRJ != nil && *self.OriginalRJ == id:
			return &RelationViolation{
				Code:    CodeRelationSelfReference,
				WorkID:  id,
				Message: "a work cannot be its own original",
			}
		}
		// An original chain: this work names an original that itself names an
		// original, so neither is the root of the family. A number without a
		// report leaves its unknown link unknown.
		if self.OriginalRJ != nil {
			if upstream, reported := ev.Self[*self.OriginalRJ]; reported && upstream.OriginalRJ != nil {
				return &RelationViolation{
					Code:    CodeRelationOriginalChain,
					WorkID:  id,
					Message: "the named original is itself a translation, which makes the original chain ambiguous",
				}
			}
		}
	}
	if cycle := findParentCycle(resolution.Relations); cycle != nil {
		return &RelationViolation{
			Code:    CodeRelationCycle,
			WorkID:  cycle[0],
			Message: "the parent relations form a cycle",
		}
	}
	return nil
}

// sortedSelfIDs lists the works that reported, in a stable order, so the
// refusal a caller sees does not depend on map iteration.
func (ev Evidence) sortedSelfIDs() []WorkCode {
	ids := make([]WorkCode, 0, len(ev.Self))
	for id := range ev.Self {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// findParentCycle reports whether following the resolved parent edges from any
// work comes back to where it started.
func findParentCycle(relations map[WorkCode]ResolvedRelation) []WorkCode {
	const (
		unvisited = iota
		onStack
		finished
	)
	state := map[WorkCode]int{}
	path := []WorkCode{}

	var walk func(WorkCode) bool
	walk = func(id WorkCode) bool {
		switch state[id] {
		case onStack:
			return true
		case finished:
			return false
		}
		state[id] = onStack
		path = append(path, id)
		if relation, known := relations[id]; known && relation.ParentID != nil {
			if walk(*relation.ParentID) {
				return true
			}
		}
		path = path[:len(path)-1]
		state[id] = finished
		return false
	}

	ids := make([]WorkCode, 0, len(relations))
	for id := range relations {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	if slices.ContainsFunc(ids, walk) {
		return path
	}
	return nil
}
