package workset_test

import (
	"testing"

	"github.com/Clementine33/preen/backend/internal/inventory"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// TestStaleValidationIgnoresNonAudioChanges covers the revision validation
// contract: only recognized audio changes make a revision stale; sidecars and
// added non-audio files never do.
func TestStaleValidationIgnoresNonAudioChanges(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	f.insertNonAudioEntry("/music/albumA/cover.jpg", "/music/albumA", 2048, 1000)
	ws := f.createCurrent("测试正交", ids...)

	gen := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	if gen.Status != "completed" {
		t.Fatalf("generation: %+v", gen)
	}
	rev := f.operation(ws.WorksetID).CurrentRevision
	if rev == nil {
		t.Fatal("expected a current revision")
	}
	if rev.ValidationState != workset.ValidationValid || rev.Stale == nil || *rev.Stale {
		t.Fatalf("expected a valid, non-stale revision, got state=%q stale=%v", rev.ValidationState, rev.Stale)
	}

	detail, revisionErr := f.svc.GetRevision(f.ctx, ws.WorksetID, "conversion", rev.PlanID)
	if revisionErr != nil {
		t.Fatal(revisionErr)
	}
	if detail.Members[0].Conclusion.Input != "valid" || !detail.Members[0].Conclusion.HasComponents {
		t.Fatalf("initial member conclusion: %+v", detail.Members[0].Conclusion)
	}

	// Non-audio churn must not invalidate the plan.
	entry500, found500, err500 := f.repo.EntryByPath("/music/albumA/cover.jpg")
	if err500 != nil || !found500 {
		t.Fatalf("read inventory: found=%v err=%v", found500, err500)
	}
	if err := f.repo.SyncObservedInventory(
		"/music",
		nil,
		[]inventory.InventoryFile{{Path: "/music/albumA/cover.jpg", Size: entry500.Size, Mtime: entry500.Mtime + 500}},
		nil,
	); err != nil {
		t.Fatal(err)
	}
	f.insertNonAudioEntry("/music/albumA/notes.txt", "/music/albumA", 500, 1000)
	afterSidecar := f.operation(ws.WorksetID).CurrentRevision
	if afterSidecar.ValidationState != workset.ValidationValid || *afterSidecar.Stale {
		t.Fatalf("non-audio change marked the revision stale: state=%q", afterSidecar.ValidationState)
	}

	// Audio churn does.
	entry100, found100, err100 := f.repo.EntryByPath("/music/albumA/01.mp3")
	if err100 != nil || !found100 {
		t.Fatalf("read inventory: found=%v err=%v", found100, err100)
	}
	if err := f.repo.SyncObservedInventory(
		"/music",
		nil,
		[]inventory.InventoryFile{{Path: "/music/albumA/01.mp3", Size: entry100.Size, Mtime: entry100.Mtime + 100}},
		nil,
	); err != nil {
		t.Fatal(err)
	}
	afterAudio := f.operation(ws.WorksetID).CurrentRevision
	if afterAudio.ValidationState != workset.ValidationStale || !*afterAudio.Stale {
		t.Fatalf("audio change must mark the revision stale: state=%q", afterAudio.ValidationState)
	}
	detail, revisionErr = f.svc.GetRevision(f.ctx, ws.WorksetID, "conversion", rev.PlanID)
	if revisionErr != nil {
		t.Fatal(revisionErr)
	}
	if detail.Members[0].Conclusion.Input != "stale" {
		t.Fatalf("member input = %q, want stale", detail.Members[0].Conclusion.Input)
	}
}

// TestARootOwnsExactlyItsOwnSubtree pins the path rule the inventory collection
// follows: a member folder is matched at a slash boundary, case-sensitively,
// with wildcard characters read literally. SQLite's LIKE is ASCII
// case-insensitive and treats % and _ as patterns, so collecting a root with it
// would hand /music/Rock the audio of /music/rock and let a folder named
// "100% hits" absorb a sibling's files — and, for the same reason, would not
// use the path index.
func TestARootOwnsExactlyItsOwnSubtree(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("Rock", "rock", "a", "ab", "100% hits", "1000 hits", "under_score")
	f.insertAudioEntry("/music/rock/01.mp3", "/music/rock", 1024, 1000)
	f.insertAudioEntry("/music/ab/01.mp3", "/music/ab", 1024, 1000)
	f.insertAudioEntry("/music/1000 hits/01.mp3", "/music/1000 hits", 1024, 1000)
	f.insertAudioEntry("/music/under_score/01.mp3", "/music/under_score", 1024, 1000)
	ws := f.createCurrent("范围精确", ids...)

	gen := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	if gen.Status != "completed" {
		t.Fatalf("generation: %+v", gen)
	}
	planID := f.operation(ws.WorksetID).CurrentRevision.PlanID

	for _, root := range []string{
		"/music/rock",
		"/music/ab",
		"/music/1000 hits",
		"/music/under_score",
	} {
		var count int
		f.queryInt(
			&count,
			"SELECT COUNT(*) FROM plan_roots WHERE plan_id = ? AND root_path = ? AND entry_count > 0",
			planID,
			root,
		)
		if count != 1 {
			t.Errorf("%s must own its own audio, entry_count>0 matched %d", root, count)
		}
	}
	for _, root := range []string{"/music/Rock", "/music/a", "/music/100% hits"} {
		var count int
		f.queryInt(
			&count,
			"SELECT COUNT(*) FROM plan_roots WHERE plan_id = ? AND root_path = ? AND entry_count > 0",
			planID,
			root,
		)
		if count != 0 {
			t.Errorf("%s absorbed another folder's audio", root)
		}
	}
}

// TestMissingRootIsBlockedButNotStaleUntilAudioAppears covers the missing-root
// accounting: a member folder absent from the inventory is a blocked fact of
// the revision, not a stale input, until audio actually appears there.
func TestMissingRootIsBlockedButNotStaleUntilAudioAppears(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumMissing")
	ws := f.createCurrent("缺失根测试", ids...)

	// The directory is gone from the disk, so the next scan of the library
	// drops its subtree from the inventory. The record keeps its member: the
	// scope is fixed, the facts about it are not.
	rows, err := f.repo.ListEntriesUnderPath("/music/albumMissing")
	if err != nil {
		t.Fatal(err)
	}
	var removed []string
	for _, row := range rows {
		removed = append(removed, row.Path)
	}
	if syncErr := f.repo.SyncObservedInventory("/music", removed, nil, nil); syncErr != nil {
		t.Fatal(syncErr)
	}

	gen := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	if gen.Status != "completed" {
		t.Fatalf("generation: %+v", gen)
	}
	rev := f.operation(ws.WorksetID).CurrentRevision
	if rev == nil {
		t.Fatal("expected a current revision")
	}
	detail, err := f.svc.GetRevision(f.ctx, ws.WorksetID, "conversion", rev.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Members[0].Conclusion.Input != "missing" {
		t.Fatalf("member input = %q, want missing", detail.Members[0].Conclusion.Input)
	}
	if rev.Counts.Blocked == 0 {
		t.Fatalf("missing root must count as blocked: %+v", rev.Counts)
	}
	if rev.ValidationState != workset.ValidationValid || *rev.Stale {
		t.Fatalf("missing root with unchanged inventory must not be stale: state=%q", rev.ValidationState)
	}

	f.insertAudioEntry("/music/albumMissing/01.mp3", "/music/albumMissing", 1024, 1000)
	after := f.operation(ws.WorksetID).CurrentRevision
	if after.ValidationState != workset.ValidationStale || !*after.Stale {
		t.Fatalf("after audio appears the revision must be stale: state=%q", after.ValidationState)
	}
}

// TestOnlyTheCurrentPlanSurvivesTheNextGeneration covers generating a new
// plan publishes it and retires the one it replaced, in that order — a record
// keeps one plan, and a failed or canceled generation never touches it.
func TestOnlyTheCurrentPlanSurvivesTheNextGeneration(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	ws := f.createCurrent("当前计划", ids...)
	f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	firstPlan := f.operation(ws.WorksetID).CurrentRevision.PlanID

	doc := draftDoc()
	doc.NoEffectTags = []string{"第二个"}
	saved := f.saveDraft(ws.WorksetID, doc, f.operation(ws.WorksetID).Version)
	if _, err := f.svc.GetRevision(
		f.ctx,
		ws.WorksetID,
		workset.OperationTypeConversion,
		firstPlan,
	); err != nil {
		t.Fatalf("the old plan must stay readable while the new one is being generated: %v", err)
	}
	f.runGeneration(ws.WorksetID, saved.Version)

	secondPlan := f.operation(ws.WorksetID).CurrentRevision.PlanID
	if secondPlan == firstPlan {
		t.Fatal("the new generation must publish a new plan")
	}
	if _, err := f.svc.GetRevision(
		f.ctx,
		ws.WorksetID,
		workset.OperationTypeConversion,
		firstPlan,
	); err == nil {
		t.Fatal("the replaced plan must not be readable")
	}
	var plans int
	f.queryInt(&plans, "SELECT COUNT(*) FROM plans WHERE workset_id = ? AND status <> 'retired'", ws.WorksetID)
	if plans != 1 {
		t.Fatalf("a record must keep exactly one plan, found %d", plans)
	}
}
