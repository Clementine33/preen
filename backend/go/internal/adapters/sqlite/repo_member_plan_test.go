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
		{RootIndex: 2, RootPath: "/music/a"},
		{RootIndex: 7, RootPath: "/music/b"},
		// Duplicate paths keep the first frozen root's ownership.
		{RootIndex: 8, RootPath: "/music/a"},
	}
	components := []workset.PlanComponentRecord{
		{ComponentIndex: 0, ComponentID: "same-id", RootIndex: 2, OutcomeJSON: `{"file":"a/keep.wav"}`},
		{ComponentIndex: 1, ComponentID: "same-id", RootIndex: 7, OutcomeJSON: `{"file":"b/private.wav"}`},
		{ComponentIndex: 2, ComponentID: "second", RootIndex: 2, OutcomeJSON: `{"file":"a/convert.flac"}`},
		{ComponentIndex: 3, ComponentID: "shadow", RootIndex: 8, OutcomeJSON: `{"file":"shadow/private.wav"}`},
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
	// A second plan reuses the root index and path: ownership includes plan id.
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
  SELECT root_index FROM plan_roots WHERE plan_id='p1' AND root_path='/music/a'`, "idx_plan_roots_path")
	assertCascadeIndex(t, repo.DB(), `EXPLAIN QUERY PLAN
  SELECT outcome_json FROM conversion_components WHERE plan_id='p1'
  AND root_index = (SELECT root_index FROM plan_roots WHERE plan_id='p1' AND root_path='/music/a' ORDER BY root_index LIMIT 1)
  ORDER BY component_index`, "idx_conversion_components_root")
	for _, tc := range []struct {
		path     string
		index    int
		outcomes []string
	}{
		{"/music/a", 2, []string{`{"file":"a/keep.wav"}`, `{"file":"a/convert.flac"}`}},
		{"/music/b", 7, []string{`{"file":"b/private.wav"}`}},
		{"/music/missing", 0, nil},
		{"", 0, nil},
	} {
		t.Run(tc.path, func(t *testing.T) {
			detail, err := repo.GetMemberPlanDetail("p1", tc.path)
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
	if _, err := repo.GetMemberPlanDetail("absent", "/music/a"); !errors.Is(err, workset.ErrPlanNotFound) {
		t.Fatalf("missing plan = %v", err)
	}
}
