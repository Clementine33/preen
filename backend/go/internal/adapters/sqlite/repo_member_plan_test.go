package sqlite_test

import (
	"errors"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestMemberPlanDetailFiltersRootsBeforeReadingOutcomes(t *testing.T) {
	for name, open := range map[string]func(string) (*sqlite.Repository, error){
		"development": sqlite.NewDevelopmentRepository,
		"release":     sqlite.NewRepository,
	} {
		t.Run(name, func(t *testing.T) { testMemberPlanDetail(t, open) })
	}
}

func testMemberPlanDetail(t *testing.T, open func(string) (*sqlite.Repository, error)) {
	t.Helper()
	repo, err := open(t.TempDir() + "/cache.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if _, err := repo.DB().Exec(`INSERT INTO worksets(id) VALUES('ws')`); err != nil {
		t.Fatal(err)
	}
	roots := []workset.PlanRootRecord{
		{RootIndex: 2, MemberID: "a", RootPath: "/music/a"},
		{RootIndex: 7, MemberID: "b", RootPath: "/music/a"},
		// Identical paths do not conflate distinct owners.
		// Duplicate owner IDs keep the first frozen root's ownership.
		{RootIndex: 8, MemberID: "a", RootPath: "/music/a"},
		{RootIndex: 9, RootPath: "/music/unowned"},
	}
	components := []workset.PlanComponentRecord{
		{ComponentIndex: 0, ComponentID: "same-id", RootIndex: 2, OutcomeJSON: `{"file":"a/keep.wav"}`},
		{ComponentIndex: 1, ComponentID: "same-id", RootIndex: 7, OutcomeJSON: `{"file":"b/private.wav"}`},
		{ComponentIndex: 2, ComponentID: "second", RootIndex: 2, OutcomeJSON: `{"file":"a/convert.flac"}`},
		{ComponentIndex: 3, ComponentID: "shadow", RootIndex: 8, OutcomeJSON: `{"file":"shadow/private.wav"}`},
		{ComponentIndex: 4, ComponentID: "standalone", RootIndex: 9, OutcomeJSON: `{"file":"standalone/private.wav"}`},
	}
	if err := sqlite.CreatePlanTx(
		repo.DB(),
		"p1",
		"conversion",
		1,
		"/music",
		"snapshot",
		"",
		"ws",
		nil,
		roots,
		components,
	); err != nil {
		t.Fatal(err)
	}
	// A second plan reuses the root index and member ID: ownership includes plan ID.
	if err := sqlite.CreatePlanTx(
		repo.DB(),
		"p2",
		"conversion",
		1,
		"/music",
		"other",
		"",
		"ws",
		nil,
		roots,
		components[1:2],
	); err != nil {
		t.Fatal(err)
	}
	assertCascadeIndex(t, repo.DB(), `EXPLAIN QUERY PLAN
  SELECT root_index FROM plan_roots WHERE plan_id='p1' AND member_id='a'`, "idx_plan_roots_member")
	assertCascadeIndex(t, repo.DB(), `EXPLAIN QUERY PLAN
  SELECT outcome_json FROM conversion_components WHERE plan_id='p1'
  AND root_index = (SELECT root_index FROM plan_roots WHERE plan_id='p1' AND member_id='a' ORDER BY root_index LIMIT 1)
  ORDER BY component_index`, "idx_conversion_components_root")
	for _, tc := range []struct {
		memberID string
		index    int
		outcomes []string
	}{
		{"a", 2, []string{`{"file":"a/keep.wav"}`, `{"file":"a/convert.flac"}`}},
		{"b", 7, []string{`{"file":"b/private.wav"}`}},
		{"missing", 0, nil},
		{"", 0, nil},
	} {
		t.Run(tc.memberID, func(t *testing.T) {
			detail, err := repo.GetMemberPlanDetail("p1", tc.memberID)
			if err != nil {
				t.Fatal(err)
			}
			if len(detail.Components) != len(tc.outcomes) {
				t.Fatalf("components = %+v", detail.Components)
			}
			if len(tc.outcomes) == 0 {
				if len(detail.Roots) != 0 {
					t.Fatalf("unexpected roots: %+v", detail.Roots)
				}
				return
			}
			if len(detail.Roots) != 1 || detail.Roots[0].RootIndex != tc.index {
				t.Fatalf("roots = %+v", detail.Roots)
			}
			for i, outcome := range tc.outcomes {
				if detail.Components[i].RootIndex != tc.index || detail.Components[i].OutcomeJSON != outcome {
					t.Fatalf("component %d = %+v", i, detail.Components[i])
				}
			}
		})
	}
	if _, err := repo.GetMemberPlanDetail("absent", "a"); !errors.Is(err, workset.ErrPlanNotFound) {
		t.Fatalf("missing plan = %v", err)
	}
}
