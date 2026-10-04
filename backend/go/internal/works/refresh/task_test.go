package refresh_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/onsei/organizer/backend/internal/works/coverdelivery"
	"github.com/onsei/organizer/backend/internal/works/refresh"
	"github.com/onsei/organizer/backend/internal/workset"
)

type taskCatalog map[string]coverdelivery.Target

func (c taskCatalog) CoverTargets(_ context.Context, _ string, _ []string) (map[string]coverdelivery.Target, error) {
	return c, nil
}
func TestMetadataPlanKeepsFrozenCopyTargetsAfterRebinding(t *testing.T) {
	root := t.TempDir()
	a := filepath.ToSlash(filepath.Join(root, "one", "RJ00000001 a"))
	b := filepath.ToSlash(filepath.Join(root, "two", "RJ00000001 b"))
	bindings := taskCatalog{
		a: {CopyID: "a", WorkCode: "RJ00000001", LibraryRoot: root, RelPath: "one/RJ00000001 a"},
		b: {CopyID: "b", WorkCode: "RJ00000001", LibraryRoot: root, RelPath: "two/RJ00000001 b"},
	}
	task := refresh.NewTask(bindings, func(string) error { return nil })
	members := []*workset.WorksetMember{
		{MemberID: "a", RelPath: "one/RJ00000001 a", FolderPath: a},
		{MemberID: "b", RelPath: "two/RJ00000001 b", FolderPath: b},
	}
	raw, _, _ := task.SeedDraft(nil)
	plan, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{LibraryRoot: root, RawDraft: raw, Members: members},
	)
	if err != nil {
		t.Fatal(err)
	}
	if task.RequiresAudio() || len(plan.Units) != 2 || plan.RootPath != root {
		t.Fatalf("copy scope lost: %+v", plan)
	}
	for _, unit := range plan.Units {
		var outcome refresh.Outcome
		if err = json.Unmarshal(unit.Payload, &outcome); err != nil {
			t.Fatal(err)
		}
		if outcome.Target.Identity != "" || outcome.Target.WorkCode != "RJ00000001" {
			t.Fatalf("target: %+v", outcome)
		}
	}
	var summary struct {
		WorkCount int `json:"work_count"`
	}
	if err = json.Unmarshal(plan.Summary, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.WorkCount != 1 {
		t.Fatalf("work not deduplicated: %s", plan.Summary)
	}
	// Binding drift does not replace the frozen targets or stale directory inputs.
	delete(bindings, b)
	facts := workset.RevisionFacts{
		DraftSnapshot: raw,
		Members:       members,
		Detail: &workset.PlanDetail{
			Plan:  workset.Plan{TaskSchemaVersion: 1, RootPath: root},
			Steps: []workset.PlanStepRecord{{PolicySchemaVersion: 1, PolicyJSON: string(plan.Payload)}},
		},
	}
	for _, r := range plan.Roots {
		facts.Detail.Roots = append(
			facts.Detail.Roots,
			workset.PlanRootRecord{
				RootIndex:            r.Index,
				RootPath:             r.Path,
				RootIdentity:         "legacy-identity",
				InventoryFingerprint: "legacy-binding-fingerprint",
			},
		)
	}
	for _, unit := range plan.Units {
		facts.Detail.Components = append(facts.Detail.Components, workset.PlanComponentRecord{
			ComponentIndex: unit.Index,
			RootIndex:      unit.RootIndex,
			ComponentID:    unit.ID,
			OutcomeJSON:    string(unit.Payload),
		})
	}
	health, err := task.EvaluateRevision(facts)
	if err != nil {
		t.Fatal(err)
	}
	if health.InputMoved {
		t.Fatal("rebinding invalidated frozen targets")
	}
	frozen, _, err := task.FreezeExecution(facts)
	if err != nil {
		t.Fatal(err)
	}
	var outcome refresh.Outcome
	if err = json.Unmarshal(frozen.Units[1].Payload, &outcome); err != nil {
		t.Fatal(err)
	}
	if outcome.Target.CopyID != "b" || outcome.Target.WorkCode != "RJ00000001" {
		t.Fatalf("reselected target: %+v", outcome)
	}
}

func TestMetadataDraftExclusionAndDynamicCoverPolicy(t *testing.T) {
	root := t.TempDir()
	folder := filepath.ToSlash(filepath.Join(root, "unbound"))
	task := refresh.NewTask(taskCatalog{}, func(string) error { return nil })
	members := []*workset.WorksetMember{{MemberID: "one", RelPath: "unbound", FolderPath: folder}}
	raw := []byte(
		`{"schema_version":1,"common":{"mode":"refresh_dynamic","locale":"ja-jp","download_cover":true},"members":[{"member_id":"one","excluded":true}]}`,
	)
	normalized, _, _, err := task.NormalizeDraft(raw, members)
	if err != nil {
		t.Fatal(err)
	}
	var draft refresh.Draft
	if err = json.Unmarshal(normalized, &draft); err != nil || draft.Common.DownloadCover {
		t.Fatalf("dynamic cover policy: %s %v", normalized, err)
	}
	plan, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{LibraryRoot: root, RawDraft: normalized, Members: members},
	)
	if err != nil || len(plan.Units) != 0 {
		t.Fatalf("excluded member planned: %+v %v", plan, err)
	}
	defaultDraft, _, _ := task.SeedDraft(nil)
	plan, err = task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{LibraryRoot: root, RawDraft: defaultDraft, Members: members},
	)
	if err != nil || len(plan.Units) != 1 || plan.Units[0].ReasonCode != "NO_WORK_BINDING" {
		t.Fatalf("unbound: %+v %v", plan, err)
	}
}
