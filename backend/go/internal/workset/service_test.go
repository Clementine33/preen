package workset_test

import (
	"strconv"
	"testing"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// TestCreateEstablishesConversionOperation covers creating the current
// record materializes the requested operation only, and the operation starts
// unplanned and addressable.
func TestCreateEstablishesConversionOperation(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA", "albumB")
	res, err := f.svc.CreateCurrentWorkset(f.ctx, workset.CreateCurrentRequest{
		LibraryID: "lib-1", Title: "夏季整理", OperationType: workset.OperationTypeConversion,
		Selection: f.selection("lib-1", ids), IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatalf("CreateCurrentWorkset: %v", err)
	}
	ws := res.Workset
	if !res.Created {
		t.Fatal("expected created=true")
	}
	if ws.Version != 1 {
		t.Fatalf("workset version = %d, want 1", ws.Version)
	}
	if len(ws.Members) != 2 {
		t.Fatalf("members = %+v", ws.Members)
	}
	op := f.operation(ws.WorksetID)
	if op.PlanningState != workset.PlanningUnplanned {
		t.Fatalf("planning_state = %q, want unplanned", op.PlanningState)
	}
	if op.Version != 1 {
		t.Fatalf("operation version = %d, want 1", op.Version)
	}

	// The seeded draft is generatable without any edit (mode + tags + outputs).
	stored := f.draft(ws.WorksetID)
	if mustDraft(t, stored).Mode != "available_sources" {
		t.Fatalf("seeded mode = %q", mustDraft(t, stored).Mode)
	}
	if len(mustDraft(t, stored).NoEffectTags) == 0 {
		t.Fatal("seeded tags must come from config.json")
	}
	if len(mustDraft(t, stored).Members) != 0 {
		t.Fatalf("seeded draft must be sparse: %+v", mustDraft(t, stored).Members)
	}

	// A replay returns the same record rather than a second one.
	replay, err := f.svc.CreateCurrentWorkset(f.ctx, workset.CreateCurrentRequest{
		LibraryID: "lib-1", Title: "夏季整理", OperationType: workset.OperationTypeConversion,
		Selection: f.selection("lib-1", ids), IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Created || replay.Workset.WorksetID != ws.WorksetID {
		t.Fatalf("replay: created=%v id=%s", replay.Created, replay.Workset.WorksetID)
	}
	if res.Recorded != 2 || len(res.Skipped) != 0 {
		t.Fatalf("recorded=%d skipped=%+v", res.Recorded, res.Skipped)
	}

	// Unimplemented operation types have no addressable state.
	if _, err := f.svc.GetOperation(f.ctx, ws.WorksetID, "rename"); err == nil {
		t.Fatal("unimplemented operation type must not resolve")
	} else if werr, ok := workset.AsError(err); !ok || werr.Code != "UNKNOWN_OPERATION_TYPE" {
		t.Fatalf("want UNKNOWN_OPERATION_TYPE, got %v", err)
	}
}

// TestCreateCurrentSkipsUnusableFolders covers the scope reporting: an id
// that cannot join is left out with its reason instead of being silently
// dropped or silently kept, and a selection with nothing usable creates no
// record at all.
func TestCreateCurrentSkipsUnusableFolders(t *testing.T) {
	f := newFixture(t)
	f.standardLibrary("albumA")
	f.insertDir("/music", "nested/child", true)
	albumA := f.selection("lib-1", []string{"albumA"}).DirIDs[0]
	nested := f.selection("lib-1", []string{"nested/child"}).DirIDs[0]

	res, err := f.svc.CreateCurrentWorkset(f.ctx, workset.CreateCurrentRequest{
		LibraryID:      "lib-1",
		OperationType:  workset.OperationTypeConversion,
		Selection:      workset.Selection{DirIDs: []string{albumA, "nope", albumA, nested}},
		IdempotencyKey: "idem-skip",
	})
	if err != nil {
		t.Fatalf("CreateCurrentWorkset: %v", err)
	}
	if res.Recorded != 1 || !res.Created {
		t.Fatalf("recorded=%d created=%v", res.Recorded, res.Created)
	}
	if len(res.Workset.Members) != 1 || res.Workset.Members[0].RelPath != "albumA" {
		t.Fatalf("members = %+v", res.Workset.Members)
	}
	want := map[string]string{
		albumA: workset.SkipDuplicate,
		"nope": workset.SkipUnknownDir,
		nested: workset.SkipUnknownDir, // not a work root
	}
	if len(res.Skipped) != len(want) {
		t.Fatalf("skipped = %+v, want %v", res.Skipped, want)
	}
	for _, skipped := range res.Skipped {
		if want[skipped.DirID] != skipped.Reason {
			t.Errorf("skipped[%q] = %q, want %q", skipped.DirID, skipped.Reason, want[skipped.DirID])
		}
	}

	// Nothing usable: no record is created, and the refusal names why.
	_, err = f.svc.CreateCurrentWorkset(f.ctx, workset.CreateCurrentRequest{
		LibraryID:      "lib-1",
		OperationType:  workset.OperationTypeConversion,
		Selection:      workset.Selection{DirIDs: []string{"nope"}},
		IdempotencyKey: "idem-none",
	})
	if err == nil {
		t.Fatal("a selection with no usable directory must not create a record")
	}
	werr, ok := workset.AsError(err)
	if !ok || werr.Code != "NO_MEMBERS" {
		t.Fatalf("want NO_MEMBERS, got %v", err)
	}
	if len(werr.Details) != 1 || werr.Details[0] != "nope: unknown_dir" {
		t.Fatalf("the refusal must name the ids it could not use: %v", werr.Details)
	}
	// The refusal left the record that was already there untouched.
	after, _ := f.svc.GetCurrentWorkset(f.ctx, "lib-1", workset.OperationTypeConversion)
	if after == nil || after.WorksetID != res.Workset.WorksetID || len(after.Members) != 1 {
		t.Fatalf("a refused creation changed the current record: %+v", after)
	}
}

// TestCreateCurrentAllExceptResolvesByRule covers `all`: every scanned work
// root but the excluded ones, in inventory order, with unknown excluded ids
// ignored, and `recorded` reporting what the inventory held at the time.
func TestCreateCurrentAllExceptResolvesByRule(t *testing.T) {
	f := newFixture(t)
	f.standardLibrary("albumC", "albumA", "albumB")
	ids := func(rels ...string) []string { return f.selection("lib-1", rels).DirIDs }
	create := func(key string, sel workset.Selection) *workset.CreateCurrentResult {
		t.Helper()
		f.ensurePlaceholders()
		res, err := f.svc.CreateCurrentWorkset(f.ctx, workset.CreateCurrentRequest{
			LibraryID: "lib-1", OperationType: workset.OperationTypeConversion,
			Selection: sel, IdempotencyKey: key, ExpectedCurrentID: f.currentID("lib-1"),
		})
		if err != nil {
			t.Fatalf("CreateCurrentWorkset: %v", err)
		}
		f.clearPlaceholders()
		return res
	}
	paths := func(res *workset.CreateCurrentResult) []string {
		out := make([]string, 0, len(res.Workset.Members))
		for _, m := range res.Workset.Members {
			out = append(out, m.RelPath)
		}
		return out
	}

	res := create("all-1", workset.Selection{All: true, ExceptDirIDs: append(ids("albumB"), "nope")})
	if got := paths(
		res,
	); len(got) != 2 || got[0] != "albumA" || got[1] != "albumC" || res.Recorded != 2 ||
		len(res.Skipped) != 0 {
		t.Fatalf("members=%v recorded=%d skipped=%+v", got, res.Recorded, res.Skipped)
	}

	// Inventory drift between what the caller saw and the request: a fourth
	// directory appeared, and `recorded` reports it instead of failing.
	f.insertDir("/music", "albumD", true)
	res = create("all-2", workset.Selection{All: true, ExceptDirIDs: ids("albumB")})
	if got := paths(res); len(got) != 3 || res.Recorded != 3 || got[2] != "albumD" {
		t.Fatalf("members=%v recorded=%d, want the drifted three", got, res.Recorded)
	}

	// Without exceptions every work root joins.
	res = create("all-3", workset.Selection{All: true})
	if res.Recorded != 4 {
		t.Fatalf("recorded=%d, want 4", res.Recorded)
	}
}

// ambiguousRoots is a store whose inventory lists albumA twice, which is the
// state two directories claiming one id would be in: the API cannot reach it
// with real hashes, so the seam is faked.
type ambiguousRoots struct{ workset.Store }

func (a ambiguousRoots) ListLibraryWorkRoots(rootPath string) ([]string, error) {
	roots, err := a.Store.ListLibraryWorkRoots(rootPath)
	return append(roots, "albumA"), err
}

// TestCreateCurrentRefusesAnAmbiguousID covers ADR 0003: an id two directories
// claim is reported as ambiguous, never resolved to either, in both forms.
func TestCreateCurrentRefusesAnAmbiguousID(t *testing.T) {
	f := newFixture(t)
	rels := f.standardLibrary("albumA", "albumB")
	f.svc = workset.NewService(ambiguousRoots{f.repo}, 1, 1, []workset.Task{
		stubTask{kind: workset.OperationTypeConversion, seed: `{"schema_version":1}`},
	}, nil, nil)
	ids := f.selection("lib-1", rels).DirIDs
	for name, sel := range map[string]workset.Selection{
		"dir_ids": {DirIDs: ids},
		"all":     {All: true},
	} {
		f.ensurePlaceholders()
		res, err := f.svc.CreateCurrentWorkset(f.ctx, workset.CreateCurrentRequest{
			LibraryID: "lib-1", OperationType: workset.OperationTypeConversion,
			Selection: sel, IdempotencyKey: "amb-" + name, ExpectedCurrentID: f.currentID("lib-1"),
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.Recorded != 1 || res.Workset.Members[0].RelPath != "albumB" ||
			len(res.Skipped) != 1 || res.Skipped[0].DirID != ids[0] || res.Skipped[0].Reason != workset.SkipAmbiguous {
			t.Fatalf("%s: recorded=%d skipped=%+v", name, res.Recorded, res.Skipped)
		}
	}
}

// TestCreateCurrentReplaysTheSameSelection covers idempotency over the
// canonical selection: the order ids were listed in is not part of the request,
// a different selection under the same key is a conflict.
func TestCreateCurrentReplaysTheSameSelection(t *testing.T) {
	f := newFixture(t)
	rels := f.standardLibrary("albumA", "albumB")
	ids := f.selection("lib-1", rels).DirIDs
	create := func(sel workset.Selection) (*workset.CreateCurrentResult, error) {
		return f.svc.CreateCurrentWorkset(f.ctx, workset.CreateCurrentRequest{
			LibraryID: "lib-1", OperationType: workset.OperationTypeConversion,
			Selection: sel, IdempotencyKey: "replay-sel",
		})
	}
	first, err := create(workset.Selection{DirIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	again, err := create(workset.Selection{DirIDs: []string{ids[1], ids[0]}})
	if err != nil || again.Created || again.Workset.WorksetID != first.Workset.WorksetID {
		t.Fatalf("reordered replay = %+v, %v", again, err)
	}
	_, err = create(workset.Selection{All: true})
	if werr, ok := workset.AsError(err); !ok || werr.Code != "IDEMPOTENCY_KEY_REUSED" {
		t.Fatalf("all under a dir_ids key: want IDEMPOTENCY_KEY_REUSED, got %v", err)
	}
}

// TestCreateCurrentValidation covers the request-level refusals.
func TestCreateCurrentValidation(t *testing.T) {
	f := newFixture(t)
	f.standardLibrary("albumA")
	ctx := f.ctx
	base := func() workset.CreateCurrentRequest {
		return workset.CreateCurrentRequest{
			LibraryID:      "lib-1",
			OperationType:  workset.OperationTypeConversion,
			Selection:      f.selection("lib-1", []string{"albumA"}),
			IdempotencyKey: "idem-validate",
		}
	}
	code := func(err error) string {
		if werr, ok := workset.AsError(err); ok {
			return werr.Code
		}
		return ""
	}

	noFolders := base()
	noFolders.Selection = workset.Selection{}
	if _, err := f.svc.CreateCurrentWorkset(ctx, noFolders); code(err) != "INVALID_FOLDER_COUNT" {
		t.Fatalf("empty selection error = %v, want INVALID_FOLDER_COUNT", err)
	}
	tooMany := base()
	tooMany.Selection = workset.Selection{DirIDs: make([]string, workset.MaxMembers+1)}
	for i := range tooMany.Selection.DirIDs {
		tooMany.Selection.DirIDs[i] = "f" + strconv.Itoa(i)
	}
	if _, err := f.svc.CreateCurrentWorkset(ctx, tooMany); code(err) != "INVALID_FOLDER_COUNT" {
		t.Fatalf("over-limit selection error = %v, want INVALID_FOLDER_COUNT", err)
	}
	mixed := base()
	mixed.Selection = workset.Selection{All: true, DirIDs: []string{"x"}}
	if _, err := f.svc.CreateCurrentWorkset(ctx, mixed); code(err) != "INVALID_SELECTION" {
		t.Fatalf("all with dir_ids error = %v, want INVALID_SELECTION", err)
	}
	stray := base()
	stray.Selection = workset.Selection{DirIDs: []string{"x"}, ExceptDirIDs: []string{"y"}}
	if _, err := f.svc.CreateCurrentWorkset(ctx, stray); code(err) != "INVALID_SELECTION" {
		t.Fatalf("except without all error = %v, want INVALID_SELECTION", err)
	}
	unknownOp := base()
	unknownOp.OperationType = "rename"
	if _, err := f.svc.CreateCurrentWorkset(ctx, unknownOp); err == nil {
		t.Fatal("an unregistered operation type should fail")
	}
	unknownLib := base()
	unknownLib.LibraryID = "nope"
	if _, err := f.svc.CreateCurrentWorkset(ctx, unknownLib); err == nil {
		t.Fatal("an unknown library should fail")
	}
}

// TestCreateCurrentReplacesTheRecordTheCallerSaw covers the replacement rule:
// the old record, its plan and its sessions become invisible when the new one publishes.
func TestCreateCurrentReplacesTheRecordTheCallerSaw(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA", "albumB")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	first := f.createCurrent("第一版", ids...)
	generated := f.runGeneration(first.WorksetID, f.operation(first.WorksetID).Version)
	if generated.Status != "completed" {
		t.Fatalf("generation: %+v", generated)
	}
	planID := f.operation(first.WorksetID).CurrentRevision.PlanID

	second := f.createCurrent("第二版", "albumB")
	if second.WorksetID == first.WorksetID {
		t.Fatal("the replacement must be a new record")
	}
	current, err := f.svc.GetCurrentWorkset(f.ctx, "lib-1", workset.OperationTypeConversion)
	if err != nil || current == nil || current.WorksetID != second.WorksetID {
		t.Fatalf("current record = %+v err=%v", current, err)
	}
	if _, err := f.svc.GetWorkset(f.ctx, first.WorksetID); err == nil {
		t.Fatal("the replaced record must be gone")
	}
	if _, err := f.svc.GetRevision(f.ctx, first.WorksetID, workset.OperationTypeConversion, planID); err == nil {
		t.Fatal("the replaced record's plan must be gone")
	}
	var leftovers int
	f.queryInt(&leftovers, "SELECT COUNT(*) FROM plans WHERE workset_id = ? AND status <> 'retired'", first.WorksetID)
	if leftovers != 0 {
		t.Fatalf("replaced plans left behind: %d", leftovers)
	}
}

// TestCreateCurrentRefusesAStaleExpectation covers the concurrency rule: a
// caller that saw another record as current must not overwrite the newer one.
func TestCreateCurrentRefusesAStaleExpectation(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA", "albumB")
	first := f.createCurrent("第一版", ids...)

	// A second client still believes there is no record.
	f.ensurePlaceholders()
	_, err := f.svc.CreateCurrentWorkset(f.ctx, workset.CreateCurrentRequest{
		LibraryID:      "lib-1",
		OperationType:  workset.OperationTypeConversion,
		Selection:      f.selection("lib-1", []string{"albumB"}),
		IdempotencyKey: "idem-stale",
	})
	if err == nil {
		t.Fatal("a stale expectation must be refused")
	}
	if werr, ok := workset.AsError(err); !ok || werr.Code != "RECORD_REPLACED" {
		t.Fatalf("want RECORD_REPLACED, got %v", err)
	}
	current, _ := f.svc.GetCurrentWorkset(f.ctx, "lib-1", workset.OperationTypeConversion)
	if current == nil || current.WorksetID != first.WorksetID {
		t.Fatalf("the newer record was overwritten: %+v", current)
	}

	// The same client retries with the id it now sees.
	replaced := f.createCurrent("第二版", "albumB")
	if replaced.WorksetID == first.WorksetID {
		t.Fatal("the retry must create the replacement")
	}
	// A client still holding the deleted first record's id must see the same
	// concurrency error, even when no global settings have been saved.
	f.ensurePlaceholders()
	_, err = f.svc.CreateCurrentWorkset(f.ctx, workset.CreateCurrentRequest{
		LibraryID:         "lib-1",
		OperationType:     workset.OperationTypeConversion,
		Selection:         f.selection("lib-1", []string{"albumA"}),
		ExpectedCurrentID: first.WorksetID,
		IdempotencyKey:    "idem-deleted-current",
	})
	if werr, ok := workset.AsError(err); !ok || werr.Code != "RECORD_REPLACED" {
		t.Fatalf("stale deleted id: want RECORD_REPLACED, got %v", err)
	}
}

// TestCreateCurrentRefusesABusyRecord covers a record with a queued or
// running session is never replaced, and it keeps its plan and members.
func TestCreateCurrentRefusesABusyRecord(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA", "albumB")
	workset.RegisterTasksForTest(f.svc, []workset.Task{
		stubTask{kind: workset.OperationTypeConversion, seed: `{"schema_version":1}`},
		stubTask{kind: "rename", seed: `{"schema_version":1}`},
	})
	first := f.createCurrent("占用", ids...)
	// A queued session that nobody runs: the fixture's dispatcher is not
	// started, so the session stays queued for the whole test.
	if _, err := f.svc.StartGeneration(f.ctx, first.WorksetID, "rename", workset.StartGenerationRequest{
		IfMatchVersion: 1, IdempotencyKey: "busy-rename",
	}); err != nil {
		t.Fatal(err)
	}

	f.ensurePlaceholders()
	_, err := f.svc.CreateCurrentWorkset(f.ctx, workset.CreateCurrentRequest{
		LibraryID:         "lib-1",
		OperationType:     workset.OperationTypeConversion,
		Selection:         f.selection("lib-1", []string{"albumB"}),
		ExpectedCurrentID: first.WorksetID,
		IdempotencyKey:    "idem-busy",
	})
	if err == nil {
		t.Fatal("replacing a busy record must be refused")
	}
	if werr, ok := workset.AsError(err); !ok || werr.Code != "RECORD_BUSY" {
		t.Fatalf("want RECORD_BUSY, got %v", err)
	}
	current, _ := f.svc.GetCurrentWorkset(f.ctx, "lib-1", workset.OperationTypeConversion)
	if current == nil || current.WorksetID != first.WorksetID || len(current.Members) != 2 {
		t.Fatalf("the busy record changed: %+v", current)
	}
}

// TestCreateCurrentRejectsAReusedKeyWithAnotherRequest covers the key rule: an
// idempotency key belongs to the request that used it.
func TestCreateCurrentRejectsAReusedKeyWithAnotherRequest(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA", "albumB")
	f.createCurrent("キー", ids...)

	f.ensurePlaceholders()
	_, err := f.svc.CreateCurrentWorkset(f.ctx, workset.CreateCurrentRequest{
		LibraryID:      "lib-1",
		OperationType:  workset.OperationTypeConversion,
		Selection:      f.selection("lib-1", []string{"albumB"}),
		IdempotencyKey: "create-キー",
	})
	if err == nil {
		t.Fatal("the same key with a different request must conflict")
	}
	if werr, ok := workset.AsError(err); !ok || werr.Code != "IDEMPOTENCY_KEY_REUSED" {
		t.Fatalf("want IDEMPOTENCY_KEY_REUSED, got %v", err)
	}
}

// TestRecordScopeIsPerLibraryAndOperation covers the ownership rule: one record per
// (library, operation), and another library keeps its own.
func TestRecordScopeIsPerLibraryAndOperation(t *testing.T) {
	f := newFixture(t)
	f.standardLibrary("albumA")
	f.insertLibrary("lib-2", "/music2")
	f.insertDir("/music2", "albumC", true)

	first := f.createCurrent("一库", "albumA")
	second := f.createCurrentFor("lib-2", "二库", "albumC")
	if first.WorksetID == second.WorksetID {
		t.Fatal("records of different libraries must be different records")
	}
	one, _ := f.svc.GetCurrentWorkset(f.ctx, "lib-1", workset.OperationTypeConversion)
	two, _ := f.svc.GetCurrentWorkset(f.ctx, "lib-2", workset.OperationTypeConversion)
	if one == nil || one.WorksetID != first.WorksetID || two == nil || two.WorksetID != second.WorksetID {
		t.Fatalf("current records: %+v / %+v", one, two)
	}
	// A library with no record answers "none", which is not an error.
	none, err := f.svc.GetCurrentWorkset(f.ctx, "lib-1", "rename")
	if err == nil {
		t.Fatalf("an unregistered operation type must not resolve: %+v", none)
	}
}

// TestRenameUsesMetadataVersionOnly covers a rename advances the workset
// metadata version and leaves the operation version (and therefore any draft
// generation or execution guard) untouched.
func TestRenameUsesMetadataVersionOnly(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	ws := f.createCurrent("旧名", ids...)
	opBefore := f.operation(ws.WorksetID)

	renamed, err := f.svc.RenameWorkset(f.ctx, ws.WorksetID, workset.RenameRequest{
		Title: "新名", IfMatchVersion: ws.Version,
	})
	if err != nil {
		t.Fatalf("RenameWorkset: %v", err)
	}
	if renamed.Title != "新名" || renamed.Version != ws.Version+1 {
		t.Fatalf("rename result: title=%q version=%d", renamed.Title, renamed.Version)
	}
	if opAfter := f.operation(ws.WorksetID); opAfter.Version != opBefore.Version {
		t.Fatalf("rename advanced the operation version: %d -> %d", opBefore.Version, opAfter.Version)
	}
	// The stale metadata version is refused.
	if _, err := f.svc.RenameWorkset(f.ctx, ws.WorksetID, workset.RenameRequest{
		Title: "再改", IfMatchVersion: ws.Version,
	}); err == nil {
		t.Fatal("stale If-Match must fail")
	}
}

// TestRenameDoesNotDirtyOperationDraft covers renaming the workset never
// moves an operation from planned to needs_planning.
func TestRenameDoesNotDirtyOperationDraft(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	ws := f.createCurrent("名字", ids...)

	gen := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	if gen.Status != "completed" {
		t.Fatalf("generation: %+v", gen)
	}
	if got := f.operation(ws.WorksetID).PlanningState; got != workset.PlanningPlanned {
		t.Fatalf("planning state = %q, want planned", got)
	}

	renamed, err := f.svc.RenameWorkset(f.ctx, ws.WorksetID, workset.RenameRequest{
		Title: "改过的名字", IfMatchVersion: ws.Version,
	})
	if err != nil {
		t.Fatalf("RenameWorkset: %v", err)
	}
	_ = renamed
	if got := f.operation(ws.WorksetID).PlanningState; got != workset.PlanningPlanned {
		t.Fatalf("rename dirtied the operation: %q", got)
	}
}

// TestOrphanedWorksetIsReadOnly covers an orphaned workset stays readable
// but refuses every write, including generation, draft edits and execution.
func TestOrphanedWorksetIsReadOnly(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	ws := f.createCurrent("孤儿", ids...)
	f.exec("UPDATE worksets SET library_id = NULL WHERE id = ?", ws.WorksetID)

	if _, err := f.svc.GetWorkset(f.ctx, ws.WorksetID); err != nil {
		t.Fatalf("GetWorkset orphaned: %v", err)
	}
	if op := f.operation(ws.WorksetID); op.PlanningState != workset.PlanningOrphaned {
		t.Fatalf("orphaned operation = %+v", op)
	}
	if _, err := f.svc.RenameWorkset(f.ctx, ws.WorksetID, workset.RenameRequest{
		Title: "x", IfMatchVersion: ws.Version,
	}); err == nil {
		t.Fatal("rename orphaned should fail")
	} else if werr, ok := workset.AsError(err); !ok || werr.Code != "ORPHANED_WORKSET" {
		t.Fatalf("expected ORPHANED_WORKSET, got %v", err)
	}
	op := f.operation(ws.WorksetID)
	if _, err := f.svc.SaveDraft(
		f.ctx,
		ws.WorksetID,
		workset.OperationTypeConversion,
		workset.SaveDraftRequest{
			Document: draftJSON(t, draftDoc()), IfMatchVersion: op.Version,
		},
	); err == nil {
		t.Fatal("draft save on orphaned workset should fail")
	}
	if _, err := f.svc.StartGeneration(
		f.ctx,
		ws.WorksetID,
		workset.OperationTypeConversion,
		workset.StartGenerationRequest{
			IfMatchVersion: op.Version, IdempotencyKey: "gen-orphan",
		},
	); err == nil {
		t.Fatal("generation on orphaned workset should fail")
	}
}

// TestListWorksetsPaginationAndOrphans covers the list contract.
func TestListWorksetsPaginationAndOrphans(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertLibrary("lib-2", "/music2")
	f.insertDir("/music2", "albumC", true)
	first := f.createCurrent("一", ids...)
	f.createCurrentFor("lib-2", "二", "albumC")

	page, cursor, err := f.svc.ListWorksets(f.ctx, workset.ListQuery{Limit: 1})
	if err != nil {
		t.Fatalf("ListWorksets: %v", err)
	}
	if len(page) != 1 || cursor == "" {
		t.Fatalf("page = %d cursor = %q", len(page), cursor)
	}
	rest, next, err := f.svc.ListWorksets(f.ctx, workset.ListQuery{Limit: 10, Cursor: cursor})
	if err != nil {
		t.Fatalf("ListWorksets page 2: %v", err)
	}
	if len(rest) != 1 || next != "" {
		t.Fatalf("second page = %d next = %q", len(rest), next)
	}

	f.exec("UPDATE worksets SET library_id = NULL WHERE id = ?", first.WorksetID)
	active, _, err := f.svc.ListWorksets(f.ctx, workset.ListQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListWorksets active: %v", err)
	}
	for _, v := range active {
		if v.WorksetID == first.WorksetID {
			t.Fatal("orphaned workset must be excluded by default")
		}
	}
	all, _, err := f.svc.ListWorksets(f.ctx, workset.ListQuery{Limit: 10, IncludeOrphaned: true})
	if err != nil {
		t.Fatalf("ListWorksets all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("include_orphaned list = %d, want 2", len(all))
	}
	_ = ids
}
