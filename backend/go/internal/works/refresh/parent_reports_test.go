package refresh_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/Clementine33/preen/backend/internal/works"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
)

const reportStamp = "2026-09-27T10:00:00.000000000Z"

// reportSource makes fetched subjects observable while all persistence uses real SQLite.
type reportSource struct {
	mu       sync.Mutex
	reports  map[works.WorkCode]*works.ImportRelations
	failures map[works.WorkCode]bool
	calls    []works.WorkCode
}

func (s *reportSource) FetchDynamic(
	_ context.Context,
	ids []works.WorkCode,
	_ string,
) (map[works.WorkCode]works.ImportRequest, error) {
	s.mu.Lock()
	s.calls = append(s.calls, ids...)
	s.mu.Unlock()
	out := map[works.WorkCode]works.ImportRequest{}
	for _, id := range ids {
		if s.failures[id] {
			return nil, errors.New("upstream unavailable")
		}
		out[id] = works.ImportRequest{
			Relations:  s.reports[id],
			Statistics: &works.ImportStatistics{FetchedAt: reportStamp},
		}
	}
	return out, nil
}
func (*reportSource) FetchStatic(context.Context, works.WorkCode, string) (refresh.Snapshot, error) {
	return refresh.Snapshot{}, errors.New("no static snapshot")
}
func (*reportSource) FetchASMR(context.Context, works.WorkCode, string) (refresh.Snapshot, error) {
	return refresh.Snapshot{}, errors.New("no backup snapshot")
}

func parentReport(role string, parent *string) *works.ImportRelations {
	return &works.ImportRelations{
		Source:    "dlsite",
		FetchedAt: reportStamp,
		Role:      role,
		ParentRJ:  parent,
		Language:  new("CHI_HANS"),
	}
}

func TestRefreshFollowsOnlyEffectiveParentsAndPersistsOnlyReports(t *testing.T) {
	repo, cat := catalog(t, "RJ00000001")
	target, page, original := works.WorkCode("RJ00000001"), works.WorkCode("RJ00000002"), works.WorkCode("RJ00000003")
	child := parentReport("child", new(string(page)))
	child.OriginalRJ = new(string(original))
	parent := parentReport("parent", nil)
	parent.OriginalRJ, parent.ChildRJs = new(string(original)), []string{string(target), "RJ00000004"}
	source := &reportSource{reports: map[works.WorkCode]*works.ImportRelations{target: child, page: parent}}
	result, err := refresh.NewService(cat, repo, source, source).
		Refresh(t.Context(), refresh.Request{WorkCodes: []string{string(target)}, Mode: refresh.RefreshDynamic})
	if err != nil || len(result.Works[0].Issues) != 0 {
		t.Fatalf("refresh: %+v, %v", result, err)
	}
	if !slices.Equal(source.calls, []works.WorkCode{target, page}) {
		t.Fatalf("fetched family beyond parent chain: %v", source.calls)
	}
	detail, refusal := cat.Detail(target)
	if refusal != nil || detail.Relation.ParentID == nil || *detail.Relation.ParentID != page ||
		detail.Relation.OriginalID == nil ||
		*detail.Relation.OriginalID != original {
		t.Fatalf("relation: %+v, %v", detail, refusal)
	}
	states, err := repo.MetadataStates(t.Context(), []works.WorkCode{page, original})
	if err != nil || len(states[page]) != 1 || states[page][works.GroupRelations].Source != works.SourceDLsite ||
		len(states[original]) != 0 {
		t.Fatalf("ancestor metadata persisted: %+v, %v", states, err)
	}
	for _, id := range []works.WorkCode{page, original, "RJ00000004"} {
		if _, refusal := cat.Detail(id); refusal == nil || refusal.Code != works.CodeWorkNotFound {
			t.Fatalf("external work %s persisted: %v", id, refusal)
		}
	}
}

func TestRefreshIgnoresIncomingParentWhenReportIsStaleOrLowerPriority(t *testing.T) {
	for _, tc := range []struct{ name, source, stamp string }{
		{"late", "dlsite", "2026-09-26T10:00:00.000000000Z"},
		{"lower priority", "asmr_one", "2026-09-28T10:00:00.000000000Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, cat := catalog(t, "RJ00000001")
			target, page := works.WorkCode("RJ00000001"), works.WorkCode("RJ00000002")
			if _, err := cat.ImportRelationReport(target, *parentReport("child", new(string(page)))); err != nil {
				t.Fatal(err)
			}
			stale := parentReport("child", new("RJ00000004"))
			stale.Source, stale.FetchedAt = tc.source, tc.stamp
			source := &reportSource{
				reports: map[works.WorkCode]*works.ImportRelations{target: stale, page: parentReport("parent", nil)},
			}
			result, err := refresh.NewService(cat, repo, source, source).
				Refresh(t.Context(), refresh.Request{WorkCodes: []string{string(target)}, Mode: refresh.RefreshDynamic})
			if err != nil || len(result.Works[0].Outcome.Ignored) != 1 ||
				!slices.Equal(source.calls, []works.WorkCode{target, page}) {
				t.Fatalf("stale traversal: %+v, %v, calls=%v", result, err, source.calls)
			}
		})
	}
}

func TestFailedAncestorRefreshKeepsExistingEvidenceAndFollowsItsParent(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "existing"}[existing], func(t *testing.T) {
			repo, cat := catalog(t, "RJ00000001")
			target, page, upstream := works.WorkCode(
				"RJ00000001",
			), works.WorkCode(
				"RJ00000002",
			), works.WorkCode(
				"RJ00000003",
			)
			if existing {
				if _, err := cat.ImportRelationReport(
					page,
					*parentReport("parent", new(string(upstream))),
				); err != nil {
					t.Fatal(err)
				}
			}
			source := &reportSource{
				reports: map[works.WorkCode]*works.ImportRelations{
					target:   parentReport("child", new(string(page))),
					upstream: parentReport("parent", nil),
				},
				failures: map[works.WorkCode]bool{page: true},
			}
			result, err := refresh.NewService(cat, repo, source, source).
				Refresh(t.Context(), refresh.Request{WorkCodes: []string{string(target)}, Mode: refresh.RefreshDynamic})
			if err != nil || len(result.Works[0].Issues) == 0 {
				t.Fatalf("failure not reported: %+v, %v", result, err)
			}
			states, err := repo.MetadataStates(t.Context(), []works.WorkCode{page})
			if err != nil || (len(states[page]) > 0) != existing {
				t.Fatalf("failed report changed evidence: %+v, %v", states, err)
			}
			want := []works.WorkCode{target, page}
			if existing {
				want = append(want, upstream)
			}
			if !slices.Equal(source.calls, want) {
				t.Fatalf("failure lost stored parent: %v, want %v", source.calls, want)
			}
		})
	}
}

func TestFillMissingCompletesStoredParentChainWithoutRefreshingTarget(t *testing.T) {
	repo, cat := catalog(t, "RJ00000001")
	target, page := works.WorkCode("RJ00000001"), works.WorkCode("RJ00000002")
	_, err := cat.Import(target, works.ImportRequest{
		Core:       &works.ImportCore{Source: "dlsite", FetchedAt: reportStamp, Title: new("existing")},
		Statistics: &works.ImportStatistics{FetchedAt: reportStamp},
		Editions:   &works.ImportEditions{Source: "dlsite", FetchedAt: reportStamp},
		Relations:  parentReport("child", new(string(page))),
	})
	if err != nil {
		t.Fatal(err)
	}
	source := &reportSource{reports: map[works.WorkCode]*works.ImportRelations{page: parentReport("parent", nil)}}
	result, refreshErr := refresh.NewService(cat, repo, source, source).
		Refresh(t.Context(), refresh.Request{WorkCodes: []string{string(target)}, Mode: refresh.FillMissing})
	if refreshErr != nil || len(result.Works[0].Issues) != 0 || !slices.Equal(source.calls, []works.WorkCode{page}) {
		t.Fatalf("missing parent: %+v, %v, calls=%v", result, refreshErr, source.calls)
	}
}

func TestParentReportsAreDeduplicatedAcrossSelectedWorks(t *testing.T) {
	repo, cat := catalog(t, "RJ00000001", "RJ00000002")
	first, second, page := works.WorkCode("RJ00000001"), works.WorkCode("RJ00000002"), works.WorkCode("RJ00000003")
	source := &reportSource{reports: map[works.WorkCode]*works.ImportRelations{
		first: parentReport(
			"child",
			new(string(page)),
		),
		second: parentReport("child", new(string(page))),
		page:   parentReport("parent", nil),
	}}
	result, err := refresh.NewService(cat, repo, source, source).
		Refresh(t.Context(), refresh.Request{WorkCodes: []string{string(first), string(second)}, Mode: refresh.RefreshDynamic})
	if err != nil || len(result.Works) != 2 || !slices.Equal(source.calls, []works.WorkCode{first, second, page}) {
		t.Fatalf("deduplication: %+v, %v, calls=%v", result, err, source.calls)
	}
}

func TestCyclicAncestorReportIsRefusedWithoutLoopingOrLosingTargetEvidence(t *testing.T) {
	repo, cat := catalog(t, "RJ00000001")
	target, page := works.WorkCode("RJ00000001"), works.WorkCode("RJ00000002")
	source := &reportSource{reports: map[works.WorkCode]*works.ImportRelations{
		target: parentReport("child", new(string(page))), page: parentReport("parent", new(string(target))),
	}}
	result, err := refresh.NewService(cat, repo, source, source).
		Refresh(t.Context(), refresh.Request{WorkCodes: []string{string(target)}, Mode: refresh.RefreshDynamic})
	if err != nil || len(result.Works[0].Issues) != 1 || result.Works[0].Issues[0].Code != works.CodeRelationCycle ||
		!slices.Equal(source.calls, []works.WorkCode{target, page}) {
		t.Fatalf("cycle: %+v, %v, calls=%v", result, err, source.calls)
	}
	states, err := repo.MetadataStates(t.Context(), []works.WorkCode{target, page})
	if err != nil || states[target][works.GroupRelations].Source != works.SourceDLsite || len(states[page]) != 0 {
		t.Fatalf("cycle changed committed evidence: %+v, %v", states, err)
	}
}

func TestAncestorOriginalReportKeepsDeclaredChildrenWithoutFetchingEditions(t *testing.T) {
	repo, cat := catalog(t, "RJ00000001")
	target, original, sibling := works.WorkCode(
		"RJ00000001",
	), works.WorkCode(
		"RJ00000002",
	), works.WorkCode(
		"RJ00000003",
	)
	report := parentReport("original", nil)
	report.ChildRJs = []string{string(target), string(sibling)}
	source := &reportSource{reports: map[works.WorkCode]*works.ImportRelations{
		target: parentReport("child", new(string(original))), original: report,
	}}
	result, err := refresh.NewService(cat, repo, source, source).
		Refresh(t.Context(), refresh.Request{WorkCodes: []string{string(target)}, Mode: refresh.RefreshDynamic})
	if err != nil || len(result.Works[0].Issues) != 0 {
		t.Fatalf("refresh: %+v, %v", result, err)
	}
	parent, err := repo.RelationParent(t.Context(), sibling)
	if err != nil || parent == nil || *parent != original {
		t.Fatalf("original declaration lost: %v, %v", parent, err)
	}
	if !slices.Equal(source.calls, []works.WorkCode{target, original}) {
		t.Fatalf("expanded sibling: %v", source.calls)
	}
}
