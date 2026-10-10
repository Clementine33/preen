package dirrename_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/dirrename"
	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/works"
	"github.com/onsei/organizer/backend/internal/workset"
)

func fixture(t *testing.T, title string) (*sqlite.Repository, *dirrename.Task, *workset.WorksetMember) {
	t.Helper()
	root := t.TempDir()
	name := "RJ000001 old"
	if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	lib, err := repo.CreateLibrary("library", root)
	if err != nil {
		t.Fatal(err)
	}
	pipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo),
		filesystem.WalkRootEntriesParallel,
		filesystem.WalkFolderEntries,
		nil,
	)
	if _, err := pipeline.ScanRootCtx(t.Context(), root, inventory.WithLibrary(lib.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := works.NewService(repo, nil, nil, nil, "").
		Import("RJ00000001", works.ImportRequest{Core: &works.ImportCore{Source: "dlsite", FetchedAt: "2020-01-01T00:00:00.000000000Z", Title: &title}}); err != nil {
		t.Fatal(err)
	}
	return repo, dirrename.New(
		repo,
		filesystem.ReadRenameDirectory,
		dirrename.ExecutionIO{ResolveDirectory: filesystem.ResolveMember, Rename: filesystem.RenameNoReplace,
			Relocate: repo.RelocateDirectoryAs, PendingCovers: repo.HasPendingRenameCovers},
	), &workset.WorksetMember{
		MemberID:   "member-1",
		RelPath:    name,
		FolderName: name,
		FolderPath: filepath.ToSlash(filepath.Join(root, name)),
	}
}

func TestPlanSuggestsCleanNamesWithoutModifyingDirectories(t *testing.T) {
	_, task, member := fixture(t, "title / one")
	draft, _, _ := task.SeedDraft(nil)
	snapshot, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{
			LibraryRoot: filepath.ToSlash(filepath.Dir(filepath.FromSlash(member.FolderPath))),
			RawDraft:    draft,
			Members:     []*workset.WorksetMember{member},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Units) != 1 || snapshot.Units[0].Status != "ok" {
		t.Fatalf("snapshot: %+v", snapshot)
	}
	var outcome dirrename.Outcome
	if err := json.Unmarshal(snapshot.Units[0].Payload, &outcome); err != nil {
		t.Fatal(err)
	}
	if outcome.Action != "rename" || outcome.RawName != "[RJ00000001] title / one" ||
		outcome.TargetName != "[RJ00000001] title _ one" {
		t.Fatalf("outcome: %+v", outcome)
	}
	if _, err := os.Stat(filepath.FromSlash(member.FolderPath)); err != nil {
		t.Fatal("planning modified the source", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(member.FolderPath), outcome.TargetName)); !os.IsNotExist(err) {
		t.Fatal("planning created the target")
	}
	_, reasons, freezeErr := task.FreezeExecution(context.Background(), workset.RevisionFacts{})
	if freezeErr == nil || len(reasons) != 0 {
		t.Fatalf("execution: %v, %v", reasons, freezeErr)
	}
}

func TestMetadataBracesAreLiteralAndDraftsCannotOverrideMembers(t *testing.T) {
	repo, task, member := fixture(t, "literal {circle}")
	title := "literal {circle}"
	if _, err := works.NewService(repo, nil, nil, nil, "").
		Import("RJ00000001", works.ImportRequest{Core: &works.ImportCore{Source: "dlsite", FetchedAt: "2021-01-01T00:00:00.000000000Z", Title: &title, Circle: &works.ImportCircle{Name: "circle"}}}); err != nil {
		t.Fatal(err)
	}
	draft := []byte(`{"schema_version":1,"common":{"template":"[{rj}] {title} {circle}"},"members":[]}`)
	snapshot, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{
			LibraryRoot: filepath.ToSlash(filepath.Dir(filepath.FromSlash(member.FolderPath))),
			RawDraft:    draft,
			Members:     []*workset.WorksetMember{member},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var o dirrename.Outcome
	if err := json.Unmarshal(snapshot.Units[0].Payload, &o); err != nil {
		t.Fatal(err)
	}
	if o.TargetName != "[RJ00000001] literal {circle} circle" {
		t.Fatalf("metadata interpreted as a template: %+v", o)
	}
	for _, raw := range []string{
		`{"schema_version":1,"common":{"template":"{title}"},"members":[]}`,
		`{"schema_version":1,"common":{"template":"{rj} {rj}"},"members":[]}`,
		`{"schema_version":1,"common":{"template":"{rj} {unknown}"},"members":[]}`,
		`{"schema_version":1,"common":{"template":"{rj}"},"members":[{"member_id":"member-1","template":"override"}]}`,
	} {
		if err := task.ValidateDraft([]byte(raw), []*workset.WorksetMember{member}); err == nil {
			t.Errorf("accepted invalid draft %s", raw)
		}
	}
}

func detail(snapshot *workset.PlanSnapshot) *workset.PlanDetail {
	d := &workset.PlanDetail{
		Plan:  workset.Plan{RootPath: snapshot.RootPath, TaskSchemaVersion: 1},
		Steps: []workset.PlanStepRecord{{PolicySchemaVersion: 1, PolicyJSON: string(snapshot.Payload)}},
	}
	for _, r := range snapshot.Roots {
		d.Roots = append(
			d.Roots,
			workset.PlanRootRecord{
				RootIndex:            r.Index,
				RootPath:             r.Path,
				InventoryFingerprint: r.InventoryFingerprint,
				RootStatus:           r.Status,
			},
		)
	}
	for _, u := range snapshot.Units {
		d.Components = append(
			d.Components,
			workset.PlanComponentRecord{
				ComponentIndex: u.Index,
				ComponentID:    u.ID,
				RootIndex:      u.RootIndex,
				OutcomeJSON:    string(u.Payload),
				Status:         u.Status,
			},
		)
	}
	return d
}

func TestRevisionOnlyTracksUsedMetadataAndRelevantDirectories(t *testing.T) {
	repo, task, member := fixture(t, "title")
	draft, _, _ := task.SeedDraft(nil)
	snapshot, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{
			LibraryRoot: filepath.ToSlash(filepath.Dir(filepath.FromSlash(member.FolderPath))),
			RawDraft:    draft,
			Members:     []*workset.WorksetMember{member},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	facts := workset.RevisionFacts{Detail: detail(snapshot)}
	// Old plans may carry an mtime-based fingerprint; child changes are no
	// longer rename inputs and compatibility is decided from frozen proposals.
	facts.Detail.Roots[0].InventoryFingerprint = "legacy-mtime-fingerprint"
	if err = os.WriteFile(filepath.Join(member.FolderPath, "changed.wav"), []byte("media"), 0000); err != nil {
		t.Fatal(err)
	}
	svc := works.NewService(repo, nil, nil, nil, "")
	if _, err := svc.PatchUserData("RJ00000001", works.UserDataRequest{Note: new("new note")}); err != nil {
		t.Fatal(err)
	}
	title := "title"
	release := "2025-01-01"
	count := 42
	if _, err := svc.Import(
		"RJ00000001",
		works.ImportRequest{
			Core: &works.ImportCore{
				Source:      "dlsite",
				FetchedAt:   "2021-01-01T00:00:00.000000000Z",
				Title:       &title,
				ReleaseDate: &release,
			},
			Statistics: &works.ImportStatistics{FetchedAt: "2021-01-01T00:00:00.000000000Z", DLCount: &count},
		},
	); err != nil {
		t.Fatal(err)
	}
	if mkdirErr := os.Mkdir(filepath.Join(snapshot.RootPath, "unrelated"), 0o755); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	health, err := task.EvaluateRevision(context.Background(), facts)
	if err != nil || health.InputMoved {
		t.Fatalf("unused facts staled revision: %+v, %v", health, err)
	}
	if writeErr := os.WriteFile(filepath.Join(snapshot.RootPath, "[RJ00000001] title"), nil, 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	health, err = task.EvaluateRevision(context.Background(), facts)
	if err != nil || !health.InputMoved {
		t.Fatalf("occupied target did not stale revision: %+v, %v", health, err)
	}
	if err = os.Remove(filepath.Join(snapshot.RootPath, "[RJ00000001] title")); err != nil {
		t.Fatal(err)
	}
	title = "different title"
	if _, importErr := svc.Import(
		"RJ00000001",
		works.ImportRequest{
			Core: &works.ImportCore{Source: "dlsite", FetchedAt: "2022-01-01T00:00:00.000000000Z", Title: &title},
		},
	); importErr != nil {
		t.Fatal(importErr)
	}
	health, err = task.EvaluateRevision(context.Background(), facts)
	if err != nil || !health.InputMoved {
		t.Fatalf("used metadata change: %+v %v", health, err)
	}
}

func TestNamesBlockMissingMetadataOccupiedTargetsAndUnsafeLengths(t *testing.T) {
	for _, tc := range []struct{ name, title, template, target, reason string }{
		{name: "missing", title: "", template: "[{rj}] {title}", reason: "METADATA_MISSING"},
		{name: "unreported actors", title: "title", template: "[{rj}] {voice_actors}", reason: "METADATA_MISSING"},
		{name: "ambiguous code", title: "RJ000002", template: "[{rj}] {title}", reason: "WORK_CODE_AMBIGUOUS"},
		{name: "code boundary lost", title: "title", template: "x{rj}", reason: "WORK_CODE_AMBIGUOUS"},
		{name: "too long", title: strings.Repeat("声", 90), template: "[{rj}] {title}", reason: "NAME_TOO_LONG"},
		{name: "reserved", title: "title", template: "CON.{rj}", reason: "NAME_RESERVED"},
		{name: "target file", title: "title", template: "[{rj}] {title}", target: "[RJ00000001] title", reason: "TARGET_EXISTS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, task, member := fixture(t, tc.title)
			if tc.target != "" {
				if err := os.WriteFile(
					filepath.Join(filepath.Dir(member.FolderPath), tc.target),
					nil,
					0o600,
				); err != nil {
					t.Fatal(err)
				}
			}
			draft, _ := json.Marshal(
				dirrename.Draft{
					SchemaVersion: 1,
					Common:        dirrename.Common{Template: tc.template},
					Members:       []dirrename.Member{},
				},
			)
			snapshot, err := task.PlanSession(
				t.Context(),
				workset.PlanSessionInput{
					LibraryRoot: filepath.ToSlash(filepath.Dir(filepath.FromSlash(member.FolderPath))),
					RawDraft:    draft,
					Members:     []*workset.WorksetMember{member},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Units[0].ReasonCode != tc.reason {
				t.Fatalf("outcome %+v", snapshot.Units[0])
			}
		})
	}
}

func TestTwoCopiesCannotPlanTheSameTarget(t *testing.T) {
	repo, task, member := fixture(t, "title")
	root := filepath.Dir(member.FolderPath)
	other := "RJ000001 second"
	if err := os.Mkdir(filepath.Join(root, other), 0o755); err != nil {
		t.Fatal(err)
	}
	libs, err := repo.ListLibraries()
	if err != nil {
		t.Fatal(err)
	}
	pipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo),
		filesystem.WalkRootEntriesParallel,
		filesystem.WalkFolderEntries,
		nil,
	)
	if _, scanErr := pipeline.ScanRootCtx(t.Context(), root, inventory.WithLibrary(libs[0].ID)); scanErr != nil {
		t.Fatal(scanErr)
	}
	second := &workset.WorksetMember{
		MemberID:   "member-2",
		RelPath:    other,
		FolderName: other,
		FolderPath: filepath.ToSlash(filepath.Join(root, other)),
	}
	draft, _, _ := task.SeedDraft(nil)
	snapshot, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{
			LibraryRoot: filepath.ToSlash(filepath.Dir(filepath.FromSlash(member.FolderPath))),
			RawDraft:    draft,
			Members:     []*workset.WorksetMember{member, second},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range snapshot.Units {
		if unit.ReasonCode != "TARGET_COLLISION" {
			t.Fatalf("duplicate target accepted: %+v", unit)
		}
	}
	health, err := task.EvaluateRevision(context.Background(), workset.RevisionFacts{Detail: detail(snapshot)})
	if err != nil || health.InputMoved || health.UnitsBlocked {
		t.Fatalf("frozen batch collision: %+v %v", health, err)
	}
}

// namedCatalog supplies the public read port for Windows paths on any test host.
type namedCatalog struct{ bindings map[string]dirrename.Work }

func TestNestedCopiesUseTheirOwnParentForConflictsAndFreshness(t *testing.T) {
	repo, task, member := fixture(t, "title")
	root := filepath.Dir(member.FolderPath)
	for _, parent := range []string{"one", "two"} {
		if err := os.MkdirAll(filepath.Join(root, parent, member.FolderName), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	libs, err := repo.ListLibraries()
	if err != nil {
		t.Fatal(err)
	}
	pipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo),
		filesystem.WalkRootEntriesParallel,
		filesystem.WalkFolderEntries,
		nil,
	)
	if _, err = pipeline.ScanRootCtx(t.Context(), root, inventory.WithLibrary(libs[0].ID)); err != nil {
		t.Fatal(err)
	}
	members := []*workset.WorksetMember{}
	for _, parent := range []string{"one", "two"} {
		rel := parent + "/" + member.FolderName
		members = append(
			members,
			&workset.WorksetMember{
				MemberID:   parent,
				RelPath:    rel,
				FolderName: member.FolderName,
				FolderPath: filepath.ToSlash(filepath.Join(root, rel)),
			},
		)
	}
	draft, _, _ := task.SeedDraft(nil)
	snapshot, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{LibraryRoot: root, RawDraft: draft, Members: members},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range snapshot.Units {
		if unit.Status != "ok" {
			t.Fatalf("cross-parent collision: %+v", unit)
		}
	}
	var outcome dirrename.Outcome
	if err = json.Unmarshal(snapshot.Units[1].Payload, &outcome); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(root, "two", outcome.TargetName), 0o755); err != nil {
		t.Fatal(err)
	}
	health, err := task.EvaluateRevision(context.Background(), workset.RevisionFacts{Detail: detail(snapshot)})
	if err != nil || !health.InputMoved || len(health.StaleRoots) != 1 || health.StaleRoots[0] != 1 {
		t.Fatalf("parent freshness = %+v, %v", health, err)
	}
}

func (c namedCatalog) RenameWorks(context.Context, string, []string) (map[string]dirrename.Work, error) {
	return c.bindings, nil
}

func TestWindowsCaseOnlyRenameAndTargetOccupancyAreBlocked(t *testing.T) {
	source := "[RJ00000001] title"
	member := &workset.WorksetMember{
		MemberID:   "m",
		FolderPath: "C:/library/" + source,
		RelPath:    source,
		FolderName: source,
	}
	catalog := namedCatalog{
		bindings: map[string]dirrename.Work{
			member.FolderPath: {Code: "RJ00000001", Values: map[string]string{"title": "TITLE"}},
		},
	}
	task := dirrename.New(catalog, func(context.Context, string) (dirrename.Directory, error) {
		return dirrename.Directory{source: {Directory: true}}, nil
	})
	draft, _, _ := task.SeedDraft(nil)
	snapshot, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{
			LibraryRoot: filepath.ToSlash(filepath.Dir(filepath.FromSlash(member.FolderPath))),
			RawDraft:    draft,
			Members:     []*workset.WorksetMember{member},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Units[0].ReasonCode != "CASE_ONLY_RENAME" {
		t.Fatalf("case-only rename accepted: %+v", snapshot.Units)
	}
}

func TestExcludedMissingMemberIsOmittedAndSourceLossStalesPlan(t *testing.T) {
	_, task, member := fixture(t, "title")
	other := &workset.WorksetMember{
		MemberID:   "missing",
		FolderPath: filepath.ToSlash(filepath.Join(filepath.Dir(member.FolderPath), "gone")),
		RelPath:    "gone",
		FolderName: "gone",
	}
	raw, _ := json.Marshal(
		dirrename.Draft{
			SchemaVersion: 1,
			Common:        dirrename.Common{Template: "[{rj}] {title}"},
			Members:       []dirrename.Member{{MemberID: other.MemberID, Excluded: true}},
		},
	)
	snapshot, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{
			LibraryRoot: filepath.ToSlash(filepath.Dir(filepath.FromSlash(member.FolderPath))),
			RawDraft:    raw,
			Members:     []*workset.WorksetMember{member, other},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Units) != 1 || snapshot.ExcludedScope != "missing" {
		t.Fatalf("exclusion: %+v", snapshot)
	}
	if removeErr := os.Remove(member.FolderPath); removeErr != nil {
		t.Fatal(removeErr)
	}
	health, err := task.EvaluateRevision(context.Background(), workset.RevisionFacts{Detail: detail(snapshot)})
	if err != nil || !health.InputMoved {
		t.Fatalf("missing source: %+v, %v", health, err)
	}
}
