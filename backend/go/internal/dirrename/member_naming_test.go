package dirrename_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/dirrename"
	"github.com/onsei/organizer/backend/internal/workset"
)

// planWith plans one member with the given per-folder draft entry.
func planWith(t *testing.T, task *dirrename.Task, m *workset.WorksetMember, entry dirrename.Member) (
	dirrename.Outcome, *workset.PlanDetail,
) {
	t.Helper()
	raw, _ := json.Marshal(dirrename.Draft{
		SchemaVersion: 1,
		Common:        dirrename.Common{Template: "[{rj}] {title}"},
		Members:       []dirrename.Member{entry},
	})
	plan, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{LibraryRoot: detailRoot(m), RawDraft: raw, Members: []*workset.WorksetMember{m}},
	)
	if err != nil {
		t.Fatal(err)
	}
	var o dirrename.Outcome
	if err := json.Unmarshal(plan.Units[0].Payload, &o); err != nil {
		t.Fatal(err)
	}
	return o, detail(plan)
}

func runUnit(t *testing.T, task *dirrename.Task, m *workset.WorksetMember, d *workset.PlanDetail) dirrename.Result {
	t.Helper()
	frozen, reasons, err := task.FreezeExecution(workset.RevisionFacts{Detail: d})
	if err != nil || len(reasons) > 0 {
		t.Fatalf("freeze: %v %v", reasons, err)
	}
	prepared, err := task.PrepareUnit(t.Context(), workset.UnitRunInput{WorksetRoot: detailRoot(m), Unit: frozen.Units[0]})
	if err != nil {
		t.Fatal(err)
	}
	res, err := prepared.Commit(t.Context())
	if err != nil || res.ErrorCode != "" {
		t.Fatalf("commit: %+v %v", res, err)
	}
	var facts dirrename.Result
	if err := json.Unmarshal(res.TaskResult, &facts); err != nil {
		t.Fatal(err)
	}
	return facts
}

func TestCustomNameReplacesTheTemplateAndKeepsTheCode(t *testing.T) {
	_, task, m := fixture(t, "title")
	o, _ := planWith(t, task, m, dirrename.Member{MemberID: m.MemberID, Name: "[RJ00000001] mine"})
	if o.Action != "rename" || o.TargetName != "[RJ00000001] mine" || o.CustomName == "" || o.RebindTo != "" {
		t.Fatalf("custom name: %+v", o)
	}
	o, _ = planWith(t, task, m, dirrename.Member{MemberID: m.MemberID, Name: "no code here"})
	if o.Action != "blocked" || o.ReasonCode != "NAME_CODE_INVALID" {
		t.Fatalf("codeless name: %+v", o)
	}
	raw := []byte(`{"schema_version":1,"common":{"template":"[{rj}] {title}"},"members":[{"member_id":"member-1","excluded":false,"name":"x"},{"member_id":"gone","excluded":false}]}`)
	if _, _, _, err := task.NormalizeDraft(raw, []*workset.WorksetMember{m}); err == nil {
		t.Fatal("unknown member accepted")
	}
	normalized, _, _, err := task.NormalizeDraft(
		[]byte(`{"schema_version":1,"common":{"template":"[{rj}] {title}"},"members":[{"member_id":"member-1","excluded":false,"name":"x"}]}`),
		[]*workset.WorksetMember{m},
	)
	if err != nil || !json.Valid(normalized) {
		t.Fatal(err)
	}
	var d dirrename.Draft
	if err := json.Unmarshal(normalized, &d); err != nil || len(d.Members) != 1 || d.Members[0].Name != "x" {
		t.Fatalf("named member dropped: %s %v", normalized, err)
	}
}

func TestCustomNameWithAnotherCodeRebindsTheCopy(t *testing.T) {
	repo, task, m := fixture(t, "title")
	o, d := planWith(t, task, m, dirrename.Member{MemberID: m.MemberID, Name: "[RJ00000002] other"})
	if o.Action != "rename" || o.RebindTo != "RJ00000002" {
		t.Fatalf("rebind: %+v", o)
	}
	facts := runUnit(t, task, m, d)
	works, err := repo.RenameWorks(t.Context(), detailRoot(m), []string{facts.TargetPath})
	if err != nil || works[facts.TargetPath].Code != "RJ00000002" {
		t.Fatalf("copy not rebound: %+v %v", works, err)
	}
	old, err := repo.RenameWorks(t.Context(), detailRoot(m), []string{m.FolderPath})
	if err != nil || len(old) != 0 {
		t.Fatalf("old path still bound: %+v %v", old, err)
	}
}

func TestBlockedFolderIsSkippedAndTheRunGoesOn(t *testing.T) {
	_, task, m := fixture(t, "title")
	o, d := planWith(t, task, m, dirrename.Member{MemberID: m.MemberID, Name: "no code here"})
	if o.Action != "blocked" {
		t.Fatalf("plan: %+v", o)
	}
	facts := runUnit(t, task, m, d)
	if facts.Renamed || facts.Skipped != "NAME_CODE_INVALID" {
		t.Fatalf("result: %+v", facts)
	}
	if _, err := os.Stat(m.FolderPath); err != nil {
		t.Fatal("blocked folder was touched", err)
	}
}

func TestHandRenamedFolderIsFollowedWithoutRescan(t *testing.T) {
	repo, task, m := fixture(t, "title")
	root := detailRoot(m)
	hand := filepath.Join(root, "RJ000001 by hand")
	if err := os.Rename(m.FolderPath, hand); err != nil {
		t.Fatal(err)
	}
	o, d := planWith(t, task, m, dirrename.Member{MemberID: m.MemberID})
	if o.ObservedName != "RJ000001 by hand" || o.Action != "rename" || o.TargetName != "[RJ00000001] title" {
		t.Fatalf("follow: %+v", o)
	}
	facts := runUnit(t, task, m, d)
	if !facts.Renamed {
		t.Fatalf("result: %+v", facts)
	}
	if _, err := os.Stat(filepath.FromSlash(facts.TargetPath)); err != nil {
		t.Fatal(err)
	}
	works, err := repo.RenameWorks(t.Context(), root, []string{facts.TargetPath})
	if err != nil || works[facts.TargetPath].Code != "RJ00000001" {
		t.Fatalf("copy did not follow: %+v %v", works, err)
	}
}

func TestHandRenameToTheTargetNameOnlyRewritesRecords(t *testing.T) {
	repo, task, m := fixture(t, "title")
	root := detailRoot(m)
	target := filepath.Join(root, "[RJ00000001] title")
	if err := os.Rename(m.FolderPath, target); err != nil {
		t.Fatal(err)
	}
	o, d := planWith(t, task, m, dirrename.Member{MemberID: m.MemberID})
	if o.Action != "noop" || o.ObservedName != "[RJ00000001] title" {
		t.Fatalf("follow noop: %+v", o)
	}
	facts := runUnit(t, task, m, d)
	if facts.Renamed {
		t.Fatalf("a record-only follow must not report a disk rename: %+v", facts)
	}
	works, err := repo.RenameWorks(t.Context(), root, []string{filepath.ToSlash(target)})
	if err != nil || works[filepath.ToSlash(target)].Code != "RJ00000001" {
		t.Fatalf("records not rewritten: %+v %v", works, err)
	}
}

func TestAmbiguousHandRenameStaysMissing(t *testing.T) {
	_, task, m := fixture(t, "title")
	root := detailRoot(m)
	if err := os.Rename(m.FolderPath, filepath.Join(root, "RJ000001 first")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "RJ000001 second"), 0o755); err != nil {
		t.Fatal(err)
	}
	o, _ := planWith(t, task, m, dirrename.Member{MemberID: m.MemberID})
	if o.ObservedName != "" || o.ReasonCode != "SOURCE_MISSING" {
		t.Fatalf("ambiguous follow: %+v", o)
	}
}

func TestVoiceActorSeparatorJoinsActorsAndIsValidated(t *testing.T) {
	_, _, m := fixture(t, "title")
	catalog := namedCatalog{map[string]dirrename.Work{
		m.FolderPath: {Code: "RJ00000001", Values: map[string]string{"title": "t"}, VoiceActors: []string{"a", "b"}},
	}}
	task := dirrename.New(catalog, filesystem.ReadRenameDirectory)
	for separator, want := range map[string]string{"": "[RJ00000001] a、b", " & ": "[RJ00000001] a & b"} {
		raw, _ := json.Marshal(dirrename.Draft{
			SchemaVersion: 1,
			Common:        dirrename.Common{Template: "[{rj}] {voice_actors}", VoiceActorSeparator: separator},
			Members:       []dirrename.Member{},
		})
		plan, err := task.PlanSession(
			t.Context(),
			workset.PlanSessionInput{LibraryRoot: detailRoot(m), RawDraft: raw, Members: []*workset.WorksetMember{m}},
		)
		if err != nil {
			t.Fatal(err)
		}
		var o dirrename.Outcome
		if err := json.Unmarshal(plan.Units[0].Payload, &o); err != nil || o.TargetName != want {
			t.Fatalf("separator %q: %+v %v", separator, o, err)
		}
	}
	for _, bad := range []string{"/", `a\b`, "x\ty", "12345678901234567"} {
		raw, _ := json.Marshal(dirrename.Draft{
			SchemaVersion: 1,
			Common:        dirrename.Common{Template: "[{rj}] {voice_actors}", VoiceActorSeparator: bad},
			Members:       []dirrename.Member{},
		})
		if err := task.ValidateDraft(raw, []*workset.WorksetMember{m}); err == nil {
			t.Errorf("accepted separator %q", bad)
		}
	}
}
