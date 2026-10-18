package works_test

import (
	"testing"

	"github.com/Clementine33/preen/backend/internal/works"
)

func code(s string) works.WorkCode {
	c, ok := works.ParseWorkCode(s)
	if !ok {
		panic("test uses an invalid work number: " + s)
	}
	return c
}

//go:fix inline

func codePtr(s string) *works.WorkCode {
	c := code(s)
	return &c
}

//go:fix inline

// evidence builds an Evidence from tidy literals so the tests read as the
// statements a source made.
func evidence(self map[string]works.RelationSelf, declared map[string][]string) works.Evidence {
	ev := works.Evidence{
		Self:             map[works.WorkCode]works.RelationSelf{},
		DeclaredChildren: map[works.WorkCode][]works.WorkCode{},
	}
	for id, report := range self {
		ev.Self[code(id)] = report
	}
	for declaring, children := range declared {
		for _, child := range children {
			ev.DeclaredChildren[code(declaring)] = append(ev.DeclaredChildren[code(declaring)], code(child))
		}
	}
	return ev
}

func relation(t *testing.T, res works.Resolution, id string) works.ResolvedRelation {
	t.Helper()
	return res.Relations[code(id)]
}

func assertRelation(t *testing.T, got works.ResolvedRelation, want works.ResolvedRelation) {
	t.Helper()
	if !equalRole(got.Role, want.Role) {
		t.Errorf("role = %v, want %v", render(got.Role), render(want.Role))
	}
	if !equalPtr(got.Language, want.Language) {
		t.Errorf("language = %v, want %v", renderStr(got.Language), renderStr(want.Language))
	}
	if !equalCodePtr(got.OriginalID, want.OriginalID) {
		t.Errorf("original = %v, want %v", renderCode(got.OriginalID), renderCode(want.OriginalID))
	}
	if !equalCodePtr(got.ParentID, want.ParentID) {
		t.Errorf("parent = %v, want %v", renderCode(got.ParentID), renderCode(want.ParentID))
	}
}

func equalRole(a, b *works.RelationRole) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalCodePtr(a, b *works.WorkCode) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func renderStr(s *string) string {
	if s == nil {
		return "NULL"
	}
	return *s
}

func render(r *works.RelationRole) string {
	if r == nil {
		return "NULL"
	}
	return string(*r)
}

func renderCode(c *works.WorkCode) string {
	if c == nil {
		return "NULL"
	}
	return string(*c)
}

// TestResolveKeepsAnOriginalsOwnRole is the regression for deriving the role
// from edges: an original has no outbound relation at all, so an edgeless
// report would resolve to nothing.
func TestResolveKeepsAnOriginalsOwnRole(t *testing.T) {
	res := works.Resolve(evidence(map[string]works.RelationSelf{
		"RJ00000001": {Role: works.RoleOriginal, Language: new("JPN")},
	}, nil))

	assertRelation(t, relation(t, res, "RJ00000001"), works.ResolvedRelation{
		Role:     new(works.RoleOriginal),
		Language: new("JPN"),
	})
}

// TestResolveKeepsAChildlessParentsOwnRole is the same regression for a parent
// whose child set is empty.
func TestResolveKeepsAChildlessParentsOwnRole(t *testing.T) {
	res := works.Resolve(evidence(map[string]works.RelationSelf{
		"RJ00000002": {Role: works.RoleParent, Language: new("JPN")},
	}, nil))

	assertRelation(t, relation(t, res, "RJ00000002"), works.ResolvedRelation{
		Role:     new(works.RoleParent),
		Language: new("JPN"),
	})
}

// TestReportedEmptyChildSetDiffersFromNotReported: "reported, with no children"
// and "has not reported" are different facts and must resolve differently.
func TestReportedEmptyChildSetDiffersFromNotReported(t *testing.T) {
	reported := works.Resolve(evidence(map[string]works.RelationSelf{
		"RJ00000003": {Role: works.RoleChild, ParentRJ: codePtr("RJ00000009")},
	}, nil))
	unreported := works.Resolve(evidence(nil, map[string][]string{
		"RJ00000009": {"RJ00000003"},
	}))

	got := relation(t, reported, "RJ00000003")
	if got.Role == nil || *got.Role != works.RoleChild {
		t.Errorf("reported work resolved to role %v, want child", render(got.Role))
	}
	if !equalCodePtr(got.ParentID, codePtr("RJ00000009")) {
		t.Errorf("reported work resolved to parent %v, want RJ00000009", renderCode(got.ParentID))
	}

	inherited := relation(t, unreported, "RJ00000003")
	if inherited.Role == nil || *inherited.Role != works.RoleChild {
		t.Errorf("declared work resolved to role %v, want child", render(inherited.Role))
	}
	if !equalCodePtr(inherited.ParentID, codePtr("RJ00000009")) {
		t.Errorf("declared work resolved to parent %v, want RJ00000009", renderCode(inherited.ParentID))
	}
}

// TestOwnReportOutranksADeclaration: a work that reported its own parent keeps
// it, and the disagreement is reported rather than applied.
func TestOwnReportOutranksADeclaration(t *testing.T) {
	res := works.Resolve(evidence(
		map[string]works.RelationSelf{
			"RJ00000004": {Role: works.RoleChild, ParentRJ: codePtr("RJ00000009")},
		},
		map[string][]string{
			"RJ00000007": {"RJ00000004"},
		},
	))

	got := relation(t, res, "RJ00000004")
	if !equalCodePtr(got.ParentID, codePtr("RJ00000009")) {
		t.Errorf("parent = %v, want the work's own report RJ00000009", renderCode(got.ParentID))
	}
	if !hasWarning(res, works.CodeRelationConflict, "RJ00000004") {
		t.Error("expected a RELATION_CONFLICT warning for the overridden declaration")
	}
}

// TestTwoDeclarersLeaveTheParentUnknown: nothing is known about which parent is
// the real one, so the parent stays unknown instead of being guessed.
func TestTwoDeclarersLeaveTheParentUnknown(t *testing.T) {
	res := works.Resolve(evidence(nil, map[string][]string{
		"RJ00000007": {"RJ00000004"},
		"RJ00000008": {"RJ00000004"},
	}))

	got := relation(t, res, "RJ00000004")
	if got.ParentID != nil {
		t.Errorf("parent = %v, want NULL", renderCode(got.ParentID))
	}
	if got.Language != nil {
		t.Errorf("language = %v, want NULL", renderStr(got.Language))
	}
	if !hasWarning(res, works.CodeRelationAmbiguous, "RJ00000004") {
		t.Error("expected a RELATION_AMBIGUOUS warning")
	}
}

// TestInheritanceFollowsTheParentChain is the parents-before-children case:
// P -> C -> J, with only P reporting anything. J must end up with P's language
// and original, which it can only reach through C's resolved values.
func TestInheritanceFollowsTheParentChain(t *testing.T) {
	res := works.Resolve(evidence(
		map[string]works.RelationSelf{
			"RJ00000001": {Role: works.RoleOriginal, Language: new("JPN")},
		},
		map[string][]string{
			"RJ00000001": {"RJ00000002"},
			"RJ00000002": {"RJ00000003"},
		},
	))

	assertRelation(t, relation(t, res, "RJ00000002"), works.ResolvedRelation{
		Role:       new(works.RoleChild),
		Language:   new("JPN"),
		OriginalID: codePtr("RJ00000001"),
		ParentID:   codePtr("RJ00000001"),
	})
	assertRelation(t, relation(t, res, "RJ00000003"), works.ResolvedRelation{
		Role:       new(works.RoleChild),
		Language:   new("JPN"),
		OriginalID: codePtr("RJ00000001"),
		ParentID:   codePtr("RJ00000002"),
	})
}

// TestOwnOriginalWinsOverTheInheritedOne: the child's own original is not
// overwritten by its parent's.
func TestOwnOriginalWinsOverTheInheritedOne(t *testing.T) {
	res := works.Resolve(evidence(
		map[string]works.RelationSelf{
			"RJ00000001": {Role: works.RoleOriginal, Language: new("JPN")},
			"RJ00000002": {Role: works.RoleChild, ParentRJ: codePtr("RJ00000001"), OriginalRJ: codePtr("RJ00000005")},
		},
		nil,
	))

	got := relation(t, res, "RJ00000002")
	if !equalCodePtr(got.OriginalID, codePtr("RJ00000005")) {
		t.Errorf("original = %v, want the work's own RJ00000005", renderCode(got.OriginalID))
	}
	// The language it did not report is still inherited.
	if !equalPtr(got.Language, new("JPN")) {
		t.Errorf("language = %v, want the inherited JPN", renderStr(got.Language))
	}
}

// TestAffectedWorksFollowsAWorksOwnParentEdge is the P1 regression: C named P
// as its parent without P declaring C, so a change to P must recompute C. A
// scope of "P plus the children P declares" would leave C on its stale values.
func TestAffectedWorksFollowsAWorksOwnParentEdge(t *testing.T) {
	before := evidence(map[string]works.RelationSelf{
		"RJ00000002": {Role: works.RoleChild, ParentRJ: codePtr("RJ00000001")},
	}, nil)
	after := evidence(map[string]works.RelationSelf{
		"RJ00000002": {Role: works.RoleChild, ParentRJ: codePtr("RJ00000001")},
		"RJ00000001": {Role: works.RoleOriginal, Language: new("JPN")},
	}, nil)

	affected := works.AffectedWorks(before, after, []works.WorkCode{code("RJ00000001")})
	if !contains(affected, "RJ00000002") {
		t.Fatalf("RJ00000002 is not in the affected set %v; it would keep its stale values", affected)
	}

	// And the recomputation actually propagates the new parent data.
	got := relation(t, works.Resolve(after), "RJ00000002")
	if !equalPtr(got.Language, new("JPN")) || !equalCodePtr(got.OriginalID, codePtr("RJ00000001")) {
		t.Errorf(
			"child did not inherit the parent's data: language=%v original=%v",
			renderStr(got.Language),
			renderCode(got.OriginalID),
		)
	}
}

// TestAffectedWorksUsesTheEvidenceFromBeforeTheChange is the other half of that
// regression: when a refresh removes a declaration, the child is gone from the
// newer graph, and only the older one still names it.
func TestAffectedWorksUsesTheEvidenceFromBeforeTheChange(t *testing.T) {
	before := evidence(nil, map[string][]string{"RJ00000001": {"RJ00000002"}})
	after := evidence(map[string]works.RelationSelf{
		"RJ00000001": {Role: works.RoleOriginal, Language: new("JPN")},
	}, nil)

	affected := works.AffectedWorks(before, after, []works.WorkCode{code("RJ00000001")})
	if !contains(affected, "RJ00000002") {
		t.Fatalf("RJ00000002 is not in the affected set %v; its inherited values would persist", affected)
	}

	// After the declaration is gone, the child resolves to nothing at all, so
	// the recomputation clears the values it inherited.
	got := relation(t, works.Resolve(after), "RJ00000002")
	if !got.IsEmpty() {
		t.Errorf("child still resolves to %v, want it cleared", got)
	}
}

// TestAffectedWorksTerminatesOnACyclicEvidenceGraph: the evidence edges may
// cycle, and the walk must still terminate and cover both members.
func TestAffectedWorksTerminatesOnACyclicEvidenceGraph(t *testing.T) {
	// A declares B a child and B declares A a child, while both reported their
	// own parent, so the effective parent edges are acyclic.
	cyclic := evidence(
		map[string]works.RelationSelf{
			"RJ00000001": {Role: works.RoleChild, ParentRJ: codePtr("RJ00000009")},
			"RJ00000002": {Role: works.RoleChild, ParentRJ: codePtr("RJ00000009")},
		},
		map[string][]string{
			"RJ00000001": {"RJ00000002"},
			"RJ00000002": {"RJ00000001"},
		},
	)
	affected := works.AffectedWorks(cyclic, cyclic, []works.WorkCode{code("RJ00000001")})
	if !contains(affected, "RJ00000002") {
		t.Errorf("affected set %v missed the work reachable only through the cycle", affected)
	}

	if violation := works.ValidateRelations(cyclic); violation != nil {
		t.Errorf("a cycle in the evidence (but not in the effective parents) was refused: %+v", violation)
	}
	res := works.Resolve(cyclic)
	for _, id := range []string{"RJ00000001", "RJ00000002"} {
		if !equalCodePtr(relation(t, res, id).ParentID, codePtr("RJ00000009")) {
			t.Errorf(
				"%s resolved to parent %v, want its own report RJ00000009",
				id,
				renderCode(relation(t, res, id).ParentID),
			)
		}
	}
}

func TestValidateRelationsRefusesSelfReference(t *testing.T) {
	for _, report := range []works.RelationSelf{
		{Role: works.RoleChild, ParentRJ: codePtr("RJ00000001")},
		{Role: works.RoleChild, OriginalRJ: codePtr("RJ00000001")},
	} {
		violation := works.ValidateRelations(evidence(map[string]works.RelationSelf{"RJ00000001": report}, nil))
		if violation == nil || violation.Code != works.CodeRelationSelfReference {
			t.Errorf("report %+v: violation = %+v, want RELATION_SELF_REFERENCE", report, violation)
		}
	}
}

func TestValidateRelationsRefusesAnOriginalChain(t *testing.T) {
	violation := works.ValidateRelations(evidence(map[string]works.RelationSelf{
		"RJ00000001": {Role: works.RoleChild, OriginalRJ: codePtr("RJ00000002")},
		"RJ00000002": {Role: works.RoleChild, OriginalRJ: codePtr("RJ00000003")},
	}, nil))
	if violation == nil || violation.Code != works.CodeRelationOriginalChain {
		t.Fatalf("violation = %+v, want RELATION_ORIGINAL_CHAIN", violation)
	}
}

func TestValidateRelationsRefusesAParentCycle(t *testing.T) {
	// Both works name each other as parent, with no report of their own to
	// outrank the declaration, so the effective parent edges do cycle.
	violation := works.ValidateRelations(evidence(nil, map[string][]string{
		"RJ00000001": {"RJ00000002"},
		"RJ00000002": {"RJ00000001"},
	}))
	if violation == nil || violation.Code != works.CodeRelationCycle {
		t.Fatalf("violation = %+v, want RELATION_CYCLE", violation)
	}
}

// TestResolveIsAFunctionOfTheEvidenceSet: the rules are deterministic in the
// evidence, so the same statements inserted in another order resolve the same.
func TestResolveIsAFunctionOfTheEvidenceSet(t *testing.T) {
	first := works.Resolve(evidence(
		map[string]works.RelationSelf{
			"RJ00000001": {Role: works.RoleOriginal, Language: new("JPN")},
			"RJ00000002": {Role: works.RoleChild, ParentRJ: codePtr("RJ00000001")},
		},
		nil,
	))
	second := works.Resolve(evidence(
		map[string]works.RelationSelf{
			"RJ00000002": {Role: works.RoleChild, ParentRJ: codePtr("RJ00000001")},
			"RJ00000001": {Role: works.RoleOriginal, Language: new("JPN")},
		},
		nil,
	))

	for _, id := range []string{"RJ00000001", "RJ00000002"} {
		assertRelation(t, relation(t, second, id), relation(t, first, id))
	}
	if len(first.Warnings) != len(second.Warnings) {
		t.Errorf("warnings differ: %v vs %v", first.Warnings, second.Warnings)
	}
}

func hasWarning(res works.Resolution, code, workID string) bool {
	for _, w := range res.Warnings {
		if w.Code == code && w.WorkID == works.WorkCode(workID) {
			return true
		}
	}
	return false
}

func contains(ids []works.WorkCode, want string) bool {
	for _, id := range ids {
		if string(id) == want {
			return true
		}
	}
	return false
}
