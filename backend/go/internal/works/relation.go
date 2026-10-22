package works

import (
	"slices"
	"sort"
)

// RelationRole is what a work says it is in the translation family.
type RelationRole string

// The roles a source relation report declares.
const (
	RoleOriginal RelationRole = "original"
	RoleParent   RelationRole = "parent"
	RoleChild    RelationRole = "child"
)

// Warning codes the relation rules emit. A warning records a disagreement that
// was accepted and resolved, not a refusal.
const (
	CodeRelationConflict  = "RELATION_CONFLICT"
	CodeRelationAmbiguous = "RELATION_AMBIGUOUS"
)

// RelationSelf is one canonical number's own independent relation report.
// Its presence in Evidence.Self means the work has
// reported; its absence means it has not, which is not the same as reporting
// no children.
type RelationSelf struct {
	Role       RelationRole
	Language   *string
	OriginalRJ *WorkCode
	ParentRJ   *WorkCode
}

// Evidence is everything the relation rules read: the reports works made about
// themselves and the child edges they declared. Both are statements a source
// made; neither is derived. The declared edges may contain a cycle - a
// declaration that a work's own report outranks is kept as evidence - so the
// rules below never assume this is a DAG.
type Evidence struct {
	// Self maps a work to its own report.
	Self map[WorkCode]RelationSelf
	// DeclaredChildren maps a declaring work to the children it named.
	DeclaredChildren map[WorkCode][]WorkCode
}

// ResolvedRelation is the relation of one work after the rules are applied:
// what work_translations stores and every reader sees.
type ResolvedRelation struct {
	Role       *RelationRole
	Language   *string
	OriginalID *WorkCode
	ParentID   *WorkCode
}

// IsEmpty reports whether the resolution says nothing at all, in which case
// there is no derived row to keep.
func (r ResolvedRelation) IsEmpty() bool {
	return r.Role == nil && r.Language == nil && r.OriginalID == nil && r.ParentID == nil
}

// RelationWarning is one accepted disagreement.
type RelationWarning struct {
	Code    string   `json:"code"`
	WorkID  WorkCode `json:"rj"`
	Message string   `json:"message"`
}

// Resolution is the outcome of applying the rules to one evidence set.
type Resolution struct {
	// Relations holds an entry for every work the resolver reached. Looking up
	// a work that is not in the map yields the zero value, meaning nothing is
	// known about its relations, which is also what an empty entry means.
	Relations map[WorkCode]ResolvedRelation
	Warnings  []RelationWarning
}

// Resolve applies the relation rules to one evidence set and returns every
// work's resolved relation. It is a pure function of the evidence: two
// catalog states with the same accepted evidence produce the same map, no
// matter in which order the evidence arrived.
//
// Inheritance follows the effective parent edges, which the import rules keep
// acyclic, so a parent is always resolved before its child. The memo and the
// in-progress guard make a cycle that slipped in anyway terminate rather than
// recurse forever.
func Resolve(ev Evidence) Resolution {
	out := Resolution{Relations: map[WorkCode]ResolvedRelation{}}
	declarers := ev.declarers()

	resolved := map[WorkCode]ResolvedRelation{}
	settled := map[WorkCode]bool{}
	active := map[WorkCode]bool{}

	var resolve func(WorkCode) ResolvedRelation
	resolve = func(id WorkCode) ResolvedRelation {
		if settled[id] {
			return resolved[id]
		}
		if active[id] {
			return ResolvedRelation{}
		}
		active[id] = true
		relation := ev.resolveOne(id, declarers, resolve, &out)
		active[id] = false
		settled[id] = true
		resolved[id] = relation
		out.Relations[id] = relation
		return relation
	}

	ids := make([]WorkCode, 0, len(ev.Self)+len(ev.DeclaredChildren))
	for id := range ev.Self {
		ids = append(ids, id)
	}
	for declaring, children := range ev.DeclaredChildren {
		// Both ends matter: the declaring work, and every child it named. A
		// child reachable only as the target of a declaration still has a
		// relation - it inherits one - even though it reported nothing itself.
		ids = append(ids, declaring)
		ids = append(ids, children...)
	}
	slices.Sort(ids)
	for _, id := range ids {
		resolve(id)
	}
	sort.Slice(out.Warnings, func(i, j int) bool {
		if out.Warnings[i].WorkID != out.Warnings[j].WorkID {
			return out.Warnings[i].WorkID < out.Warnings[j].WorkID
		}
		return out.Warnings[i].Code < out.Warnings[j].Code
	})
	return out
}

// resolveOne computes the relation of one work: its own report wins outright,
// and a work that has not reported inherits from its single declaring parent.
func (ev Evidence) resolveOne(
	id WorkCode,
	declarers map[WorkCode][]WorkCode,
	resolve func(WorkCode) ResolvedRelation,
	out *Resolution,
) ResolvedRelation {
	claims := declarers[id]

	if self, reported := ev.Self[id]; reported {
		role := self.Role
		relation := ResolvedRelation{
			Role:       &role,
			Language:   self.Language,
			OriginalID: self.OriginalRJ,
			ParentID:   self.ParentRJ,
		}
		// A declaration that disagrees with the work's own report is kept as
		// evidence but never applied: the work's own statement is the one that
		// decides. Saying so is the whole point of reporting it.
		for _, declarer := range claims {
			if relation.ParentID == nil || *relation.ParentID != declarer {
				out.Warnings = append(out.Warnings, RelationWarning{
					Code:   CodeRelationConflict,
					WorkID: id,
					Message: "another work declares this one a child, which disagrees with " +
						"this work's own relation report; the work's own report was kept",
				})
				break
			}
		}
		if relation.ParentID != nil {
			inherit(&relation, *relation.ParentID, resolve(*relation.ParentID))
		}
		return relation
	}

	switch len(claims) {
	case 0:
		// Nothing is known about this work's relations. Unknown stays unknown.
		return ResolvedRelation{}
	case 1:
		parent := claims[0]
		role := RoleChild
		relation := ResolvedRelation{Role: &role, ParentID: &parent}
		inherit(&relation, parent, resolve(parent))
		return relation
	default:
		// Two works claim the same child. Nothing is known about which one is
		// the parent, so no parent is recorded - but the role is: a declared
		// work with no report of its own is a translation of something.
		role := RoleChild
		out.Warnings = append(out.Warnings, RelationWarning{
			Code:   CodeRelationAmbiguous,
			WorkID: id,
			Message: "several works declare this one a child and it has not reported " +
				"its own relations; no parent could be determined",
		})
		return ResolvedRelation{Role: &role}
	}
}

// inherit fills a child's language and original from its resolved parent. An
// original is only borrowed when the child has none of its own: the parent's
// own original, or the parent itself when the parent is the original.
func inherit(relation *ResolvedRelation, parentID WorkCode, parent ResolvedRelation) {
	if relation.Language == nil {
		relation.Language = parent.Language
	}
	if relation.OriginalID != nil {
		return
	}
	switch {
	case parent.OriginalID != nil:
		relation.OriginalID = parent.OriginalID
	case parent.Role != nil && *parent.Role == RoleOriginal:
		original := parentID
		relation.OriginalID = &original
	}
}

// declarers returns the reverse of the declared edges, so "who says this work
// is their child" is a lookup rather than a scan.
func (ev Evidence) declarers() map[WorkCode][]WorkCode {
	reports := make(map[WorkCode][]WorkCode, len(ev.DeclaredChildren))
	for declaring, children := range ev.DeclaredChildren {
		for _, child := range children {
			reports[child] = append(reports[child], declaring)
		}
	}
	for child := range reports {
		slices.Sort(reports[child])
	}
	return reports
}

// dependents lists the works whose resolution reads this work's: the children
// it declared and the works that named it as their parent. This is the reverse
// of the dependency edges and is walked backwards to find what a change to a
// work can affect.
func (ev Evidence) dependents(id WorkCode) []WorkCode {
	found := append([]WorkCode(nil), ev.DeclaredChildren[id]...)
	for self, report := range ev.Self {
		if report.ParentRJ != nil && *report.ParentRJ == id {
			found = append(found, self)
		}
	}
	return found
}

// AffectedWorks returns every work whose resolved relation must be recomputed
// after the evidence changed.
//
// The affected set is not "the work plus the children it declares": a work that
// named a parent, without that parent declaring it, inherits from the parent
// too and must be recomputed when the parent's own report arrives. So the walk
// starts at the changed works and follows dependents backwards, and it runs over
// the union of the evidence before and after the change: a declaration that a
// refresh removed is gone from the newer graph, and only the older one still
// knows the child that has to give up its inherited values.
//
// The walk uses a visited set because the evidence edges may contain a cycle.
// The result is sorted, so the recomputation that consumes it is deterministic.
func AffectedWorks(before, after Evidence, changed []WorkCode) []WorkCode {
	seen := map[WorkCode]struct{}{}
	queue := make([]WorkCode, 0, len(changed))
	for _, id := range changed {
		if _, done := seen[id]; done {
			continue
		}
		seen[id] = struct{}{}
		queue = append(queue, id)
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, dependent := range append(before.dependents(current), after.dependents(current)...) {
			if _, done := seen[dependent]; done {
				continue
			}
			seen[dependent] = struct{}{}
			queue = append(queue, dependent)
		}
	}
	found := make([]WorkCode, 0, len(seen))
	for id := range seen {
		found = append(found, id)
	}
	slices.Sort(found)
	return found
}
