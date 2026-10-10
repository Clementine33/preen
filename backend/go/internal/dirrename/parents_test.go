package dirrename_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/dirrename"
	"github.com/onsei/organizer/backend/internal/workset"
)

func TestRenameReadsEachSelectedParentOnce(t *testing.T) {
	root := filepath.ToSlash(t.TempDir())
	members := []*workset.WorksetMember{
		{MemberID: "a", RelPath: "a", FolderPath: root + "/a"},
		{MemberID: "b", RelPath: "b", FolderPath: root + "/b"},
	}
	reads := []string{}
	catalog := namedCatalog{bindings: map[string]dirrename.Work{
		members[0].FolderPath: {Code: "RJ00000001", Values: map[string]string{"title": "a"}},
		members[1].FolderPath: {Code: "RJ00000002", Values: map[string]string{"title": "b"}},
	}}
	task := dirrename.New(catalog, func(_ context.Context, path string) (dirrename.Directory, error) {
		reads = append(reads, path)
		return dirrename.Directory{"a": {Directory: true}, "b": {Directory: true}}, nil
	})
	draft, _, _ := task.SeedDraft(nil)
	plan, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{LibraryRoot: root, RawDraft: draft, Members: members},
	)
	if err != nil || len(reads) != 1 || reads[0] != root {
		t.Fatalf("parent reads: %v %v", reads, err)
	}
	reads = nil
	health, err := task.EvaluateRevision(context.Background(), workset.RevisionFacts{Detail: detail(plan)})
	if err != nil || health.InputMoved || len(reads) != 1 || reads[0] != root {
		t.Fatalf("freshness reads: %+v %v %v", health, reads, err)
	}
}

func TestRenameMissingParentStillProducesBlockedProposal(t *testing.T) {
	root := filepath.ToSlash(t.TempDir())
	member := &workset.WorksetMember{
		MemberID:   "a",
		RelPath:    "missing/RJ00000001",
		FolderPath: root + "/missing/RJ00000001",
	}
	task := dirrename.New(namedCatalog{}, filesystem.ReadRenameDirectory)
	draft, _, _ := task.SeedDraft(nil)
	plan, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{LibraryRoot: root, RawDraft: draft, Members: []*workset.WorksetMember{member}},
	)
	if err != nil || len(plan.Units) != 1 || plan.Units[0].ReasonCode != "SOURCE_MISSING" {
		t.Fatalf("missing root: %+v %v", plan, err)
	}
}
