package workset_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/testutil/conclusionfixture"
	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestRevisionConclusionExcludedMembers(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("included", "excluded")
	f.insertAudioEntry("/music/included/track.mp3", "/music/included", 1024, 1000)
	ws := f.createCurrent("成员结论", ids...)
	excludedID := ws.Members[1].MemberID
	doc := draftDoc()
	doc.Members = []conversion.DraftMember{{MemberID: excludedID, Excluded: true}}
	saved := f.saveDraft(ws.WorksetID, doc, f.operation(ws.WorksetID).Version)
	gen := f.runGeneration(ws.WorksetID, saved.Version)
	if gen.Status != "completed" {
		t.Fatalf("generation: %+v", gen)
	}
	detail, err := f.svc.GetRevision(f.ctx, ws.WorksetID, "conversion", gen.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Members) != 2 {
		t.Fatalf("members = %d, want 2", len(detail.Members))
	}
	for _, member := range detail.Members {
		conclusion := member
		if conclusion.MemberID != member.MemberID {
			t.Fatalf("member identity lost: %+v", conclusion)
		}
		if member.MemberID != excludedID {
			continue
		}
		if !conclusion.Excluded || conclusion.Input != "none" || conclusion.HasComponents {
			t.Fatalf("excluded member conclusion = %+v", conclusion)
		}
		var task struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(conclusion.Task, &task); err != nil {
			t.Fatal(err)
		}
		if task.Status != "not_applicable" {
			t.Fatalf("excluded member task status = %q", task.Status)
		}
	}
}

type failedRevisionHealth struct{ workset.Task }

func (failedRevisionHealth) EvaluateRevision(context.Context, workset.RevisionFacts) (workset.RevisionHealth, error) {
	return workset.RevisionHealth{}, errors.New("inventory unavailable")
}

func TestRevisionConclusionRejectsFailedInputValidation(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("album")
	f.insertAudioEntry("/music/album/track.mp3", "/music/album", 1024, 1000)
	ws := f.createCurrent("输入校验", ids...)
	gen := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	svc := workset.NewService(f.repo, 1, 1, []workset.Task{failedRevisionHealth{Task: &conversion.Task{}}}, nil, nil)
	if detail, err := svc.GetRevision(f.ctx, ws.WorksetID, "conversion", gen.RevisionID); err == nil {
		t.Fatalf("failed validation returned member input conclusions: %+v", detail.Members)
	}
}

func TestRevisionConclusionUnavailableWithoutLibrary(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("included", "excluded")
	f.insertAudioEntry("/music/included/track.mp3", "/music/included", 1024, 1000)
	ws := f.createCurrent("无媒体库", ids...)
	includedID, excludedID := ws.Members[0].MemberID, ws.Members[1].MemberID
	doc := draftDoc()
	doc.Members = []conversion.DraftMember{{MemberID: excludedID, Excluded: true}}
	saved := f.saveDraft(ws.WorksetID, doc, f.operation(ws.WorksetID).Version)
	gen := f.runGeneration(ws.WorksetID, saved.Version)
	if gen.Status != "completed" {
		t.Fatalf("generation: %+v", gen)
	}
	f.exec("UPDATE worksets SET library_id = NULL WHERE id = ?", ws.WorksetID)

	detail, err := f.svc.GetRevision(f.ctx, ws.WorksetID, "conversion", gen.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{includedID: "unavailable", excludedID: "none"}
	for _, member := range detail.Members {
		if member.Input != want[member.MemberID] {
			t.Fatalf("member %s input = %q, want %q", member.MemberID, member.Input, want[member.MemberID])
		}
	}
	review, err := f.svc.GetMemberReview(f.ctx, ws.WorksetID, "conversion", gen.RevisionID, includedID)
	if err != nil {
		t.Fatal(err)
	}
	if review.Member.Conclusion.Input != "unavailable" {
		t.Fatalf("plan review input = %q, want unavailable", review.Member.Conclusion.Input)
	}
}

// TestStoredSummaryMatchesRecomputeForBlockedAndEmptyPlans covers the
// remaining shapes: a blocked component beside a changed one, and a plan with
// no components at all.
func TestStoredSummaryMatchesRecomputeForBlockedAndEmptyPlans(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("blocked", "changed", "excluded")
	// Two equivalent lossless sources of one stem block that component.
	for _, name := range []string{"a.wav", "a.WAV"} {
		inventoryfixture.SeedInventory(t, f.repo, inventoryfixture.Row{
			Path: "/music/blocked/" + name, RootPath: "/music/blocked", Size: 1024, Mtime: 1000,
			Format: "wav", CreatedAt: time.Now().Unix(),
		})
	}
	f.insertAudioEntry("/music/changed/b.mp3", "/music/changed", 1024, 1000)
	ws := f.createCurrent("摘要等价", ids...)
	gen := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	if gen.Status != "completed" {
		t.Fatalf("generation: %+v", gen)
	}
	counts := f.assertStoredSummary(gen.RevisionID)
	if counts.Blocked == 0 || counts.UnmetTargets == 0 {
		t.Fatalf("expected a blocked component and an unmet target: %+v", counts)
	}

	doc := draftDoc()
	// Only the folder without audio stays: it plans no components.
	for _, m := range ws.Members[:2] {
		doc.Members = append(doc.Members, conversion.DraftMember{MemberID: m.MemberID, Excluded: true})
	}
	saved := f.saveDraft(ws.WorksetID, doc, f.operation(ws.WorksetID).Version)
	empty := f.runGeneration(ws.WorksetID, saved.Version)
	if empty.Status != "completed" {
		t.Fatalf("generation: %+v", empty)
	}
	detail, err := f.repo.GetPlanDetail(empty.RevisionID)
	if err != nil || len(detail.Components) != 0 {
		t.Fatalf("empty plan: %+v %v", detail, err)
	}
	f.assertStoredSummary(empty.RevisionID)
}

// TestStoredConclusionsMatchReloadedPlan stages a plan whose components span
// the component staging chunk, and compares the conclusions stored at
// generation with the rule re-derived from the reloaded full plan.
func TestStoredConclusionsMatchReloadedPlan(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("big", "mixed", "excluded", "empty", "gone")
	foreign := func(path, root, format string) inventoryfixture.Row {
		return inventoryfixture.Row{
			Path: path, RootPath: root, Size: 1024, Mtime: 1000, Format: format, CreatedAt: time.Now().Unix(),
		}
	}
	// 105 components exceed the 100-row component staging chunk.
	// A foreign codec has no lossless source, so its stem stays an unmet target.
	var rows []inventoryfixture.Row
	for i := range 105 {
		rows = append(rows, foreign(fmt.Sprintf("/music/big/s%03d/t%03d.m4a", i, i), "/music/big", "m4a"))
	}
	rows = append(rows,
		foreign("/music/mixed/a/u.m4a", "/music/mixed", "m4a"),
		foreign("/music/mixed/b/t.wav", "/music/mixed", "wav"),
		foreign("/music/excluded/t.wav", "/music/excluded", "wav"),
	)
	inventoryfixture.SeedInventory(t, f.repo, rows...)
	ws := f.createCurrent("冻结结论", ids...)
	bigID, mixedID, excludedID, emptyID, goneID := ws.Members[0].MemberID, ws.Members[1].MemberID,
		ws.Members[2].MemberID, ws.Members[3].MemberID, ws.Members[4].MemberID
	gone, err := f.repo.ListEntriesUnderPath("/music/gone")
	if err != nil {
		t.Fatal(err)
	}
	var removed []string
	for _, row := range gone {
		removed = append(removed, row.Path)
	}
	if err := f.repo.SyncObservedInventory("/music", removed, nil, nil); err != nil {
		t.Fatal(err)
	}
	doc := draftDoc()
	doc.Members = []conversion.DraftMember{{MemberID: excludedID, Excluded: true}}
	saved := f.saveDraft(ws.WorksetID, doc, f.operation(ws.WorksetID).Version)
	gen := f.runGeneration(ws.WorksetID, saved.Version)
	if gen.Status != "completed" {
		t.Fatalf("generation: %+v", gen)
	}

	stored := conclusionfixture.AssertStored(t, f.repo, &conversion.Task{}, ws.WorksetID, "conversion", gen.RevisionID)
	status := func(id string) string {
		var task struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(stored[id].Task, &task); err != nil {
			t.Fatal(err)
		}
		return task.Status
	}
	detail, err := f.repo.GetPlanDetail(gen.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(detail.Components); got <= 100 {
		t.Fatalf("only %d components: the member must span a staging chunk", got)
	}
	for id, want := range map[string]string{
		bigID: "unmet", mixedID: "partially_unmet", excludedID: "not_applicable", emptyID: "not_applicable",
	} {
		if got := status(id); got != want {
			t.Fatalf("member %s status = %q, want %q", id, got, want)
		}
	}
	if !stored[bigID].HasComponents || !stored[mixedID].HasComponents || stored[goneID].HasComponents ||
		stored[excludedID].HasComponents || stored[emptyID].HasComponents || !stored[excludedID].Excluded {
		t.Fatalf("frozen facts: %+v", stored)
	}
	revision, err := f.svc.GetRevision(f.ctx, ws.WorksetID, "conversion", gen.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	wantInput := map[string]string{
		bigID: "valid", mixedID: "valid", excludedID: "none", emptyID: "valid", goneID: "missing",
	}
	for _, member := range revision.Members {
		if member.Input != wantInput[member.MemberID] || !bytes.Equal(member.Task, stored[member.MemberID].Task) {
			t.Fatalf("member %s read %+v, stored %+v", member.MemberID, member, stored[member.MemberID])
		}
	}
}
