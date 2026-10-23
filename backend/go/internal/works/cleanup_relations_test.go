package works_test

import (
	"fmt"
	"maps"
	"reflect"
	"testing"

	"github.com/Clementine33/preen/backend/internal/works"
)

func TestReportCleanupRetainsDeclarerFixedPoint(t *testing.T) {
	ev := evidence(map[string]works.RelationSelf{
		"RJ00000001": {Role: works.RoleOriginal},
		"RJ00000002": {Role: works.RoleChild, ParentRJ: codePtr("RJ00000003")},
		"RJ00000003": {Role: works.RoleOriginal},
		"RJ00000004": {Role: works.RoleOriginal},
	}, map[string][]string{"RJ00000002": {"RJ00000001", "RJ00000003", "RJ00000004"}})
	groups, err := works.PlanReportCleanup(t.Context(), ev, []works.WorkCode{code("RJ00000001")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]works.WorkCode{{code("RJ00000004")}}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("groups=%v want=%v", groups, want)
	}
}

func TestReportCleanupGroupsPreventIntermediateCycles(t *testing.T) {
	ev := evidence(map[string]works.RelationSelf{
		"RJ00000001": {Role: works.RoleChild, ParentRJ: codePtr("RJ00000002")},
		"RJ00000002": {Role: works.RoleOriginal},
		"RJ00000003": {Role: works.RoleChild, OriginalRJ: codePtr("RJ00000002")},
	}, map[string][]string{"RJ00000001": {"RJ00000002"}})
	groups, err := works.PlanReportCleanup(t.Context(), ev, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups=%v", groups)
	}
	for _, group := range groups {
		for _, id := range group {
			delete(ev.Self, id)
			delete(ev.DeclaredChildren, id)
		}
		if violation := works.ValidateRelations(ev); violation != nil {
			t.Fatalf("after %v: %+v", group, violation)
		}
	}
}

func TestReportCleanupOriginalAloneDoesNotRetainAndSnapshotProtectsNewReports(t *testing.T) {
	ev := evidence(map[string]works.RelationSelf{
		"RJ00000001": {Role: works.RoleChild, OriginalRJ: codePtr("RJ00000002")},
		"RJ00000002": {Role: works.RoleOriginal},
		"RJ00000003": {Role: works.RoleChild, ParentRJ: codePtr("RJ00000002")},
	}, nil)
	groups, err := works.PlanReportCleanup(t.Context(), ev, []works.WorkCode{code("RJ00000001")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("OriginalRJ expanded: %v", groups)
	}
	groups, err = works.PlanReportCleanup(t.Context(), ev, nil, []works.WorkCode{code("RJ00000002")})
	if err != nil || len(groups) != 0 {
		t.Fatalf("new report dependency unprotected: %v %v", groups, err)
	}
}

//nolint:gocognit // Exhaustively checks independent invariants across generated evidence and root sets.
func TestReportCleanupPreservesEveryValidThreeNodeEvidenceSet(t *testing.T) {
	ids := []works.WorkCode{code("RJ00000001"), code("RJ00000002"), code("RJ00000003")}
	checked := 0
	for state := range 125 {
		for claims := range 64 {
			ev := works.Evidence{
				Self:             map[works.WorkCode]works.RelationSelf{},
				DeclaredChildren: map[works.WorkCode][]works.WorkCode{},
			}
			value := state
			for _, id := range ids {
				choice := value % 5
				value /= 5
				if choice == 0 {
					continue
				}
				self := works.RelationSelf{Role: works.RoleOriginal}
				if choice > 1 {
					self.Role = works.RoleChild
					self.ParentRJ = new(works.WorkCode(fmt.Sprintf("RJ%08d", choice-1)))
				}
				ev.Self[id] = self
			}
			bit := 0
			for _, id := range ids {
				for _, child := range ids {
					if id == child {
						continue
					}
					if _, exists := ev.Self[id]; exists && claims&(1<<bit) != 0 {
						ev.DeclaredChildren[id] = append(ev.DeclaredChildren[id], child)
					}
					bit++
				}
			}
			if works.ValidateRelations(ev) != nil {
				continue
			}
			for _, roots := range [][]works.WorkCode{nil, {ids[0]}, {ids[1]}, {ids[2]}} {
				before := works.ResolveRoots(ev, roots)
				groups, err := works.PlanReportCleanup(t.Context(), ev, roots, nil)
				if err != nil {
					t.Fatal(err)
				}
				remaining := works.Evidence{
					Self:             maps.Clone(ev.Self),
					DeclaredChildren: maps.Clone(ev.DeclaredChildren),
				}
				for _, group := range groups {
					for _, id := range group {
						delete(remaining.Self, id)
						delete(remaining.DeclaredChildren, id)
					}
					if violation := works.ValidateRelations(remaining); violation != nil {
						t.Fatalf(
							"state=%d claims=%d roots=%v group=%v violation=%+v",
							state,
							claims,
							roots,
							group,
							violation,
						)
					}
					after := works.ResolveRoots(remaining, roots)
					if !reflect.DeepEqual(before.Relations, after.Relations) ||
						!reflect.DeepEqual(before.Warnings, after.Warnings) {
						t.Fatalf(
							"changed resolution: state=%d claims=%d roots=%v group=%v",
							state,
							claims,
							roots,
							group,
						)
					}
				}
				checked++
			}
		}
	}
	t.Logf("validated %d root/evidence combinations at every group boundary", checked)
}
