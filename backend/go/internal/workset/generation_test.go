package workset_test

import (
	"sync"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// Both requests reach the enqueue guard only after the active-session check.
func raceGenerationStarts(
	t *testing.T, f *fixture, worksetID string, keys [2]string,
) ([2]*workset.StartGenerationResult, [2]error) {
	t.Helper()
	ready := make(chan struct{})
	var arrived sync.WaitGroup
	arrived.Add(2)
	f.wireService(func(fn func() error) error {
		arrived.Done()
		<-ready
		return fn()
	})
	var results [2]*workset.StartGenerationResult
	var errs [2]error
	var done sync.WaitGroup
	done.Add(2)
	version := f.operation(worksetID).Version
	for i := range keys {
		go func(i int) {
			defer done.Done()
			results[i], errs[i] = f.svc.StartGeneration(f.ctx, worksetID, workset.OperationTypeConversion,
				workset.StartGenerationRequest{IfMatchVersion: version, IdempotencyKey: keys[i]})
		}(i)
	}
	arrived.Wait()
	close(ready)
	done.Wait()
	return results, errs
}

func TestConcurrentDifferentGenerationKeysAllowOnlyOneActiveSession(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	ws := f.createCurrent("并发规划", ids...)
	results, errs := raceGenerationStarts(t, f, ws.WorksetID, [2]string{"key-a", "key-b"})
	created, conflicts := 0, 0
	for i, err := range errs {
		if err == nil && results[i].Created {
			created++
		} else if werr, ok := workset.AsError(err); ok && werr.Code == "GENERATION_IN_PROGRESS" {
			conflicts++
		} else {
			t.Fatalf("start %d: result=%+v err=%v", i, results[i], err)
		}
	}
	if created != 1 || conflicts != 1 {
		t.Fatalf("created=%d conflicts=%d", created, conflicts)
	}
	var active int
	err := f.repo.DB().QueryRow(
		"SELECT COUNT(*) FROM plan_generations WHERE status IN ('queued','running')",
	).Scan(&active)
	if err != nil || active != 1 {
		t.Fatalf("active=%d err=%v", active, err)
	}
}

func TestConcurrentSameGenerationKeyReplaysActiveSession(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	ws := f.createCurrent("并发回放", ids...)
	results, errs := raceGenerationStarts(t, f, ws.WorksetID, [2]string{"same-key", "same-key"})
	for i, err := range errs {
		if err != nil || results[i] == nil || results[i].Generation == nil {
			t.Fatalf("start %d: result=%+v err=%v", i, results[i], err)
		}
	}
	if results[0].Created == results[1].Created ||
		results[0].Generation.GenerationID != results[1].Generation.GenerationID {
		t.Fatalf("same-key starts = %+v, %+v", results[0], results[1])
	}
}

func TestConcurrentReusedGenerationKeyIgnoresOlderFailure(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	ws := f.createCurrent("失败后竞争", ids...)
	first, err := f.svc.StartGeneration(f.ctx, ws.WorksetID, workset.OperationTypeConversion,
		workset.StartGenerationRequest{IfMatchVersion: f.operation(ws.WorksetID).Version, IdempotencyKey: "reused-key"})
	if err != nil {
		t.Fatal(err)
	}
	old := first.Generation.GenerationID
	if err := f.repo.MarkGenerationFailed(old, "FAILED", "first attempt"); err != nil {
		t.Fatal(err)
	}
	results, errs := raceGenerationStarts(t, f, ws.WorksetID, [2]string{"reused-key", "reused-key"})
	for i, err := range errs {
		if err != nil || results[i] == nil || results[i].Generation == nil ||
			results[i].Generation.GenerationID == old {
			t.Fatalf("start %d: result=%+v err=%v old=%s", i, results[i], err, old)
		}
	}
	if results[0].Generation.GenerationID != results[1].Generation.GenerationID {
		t.Fatalf("different live sessions: %+v, %+v", results[0], results[1])
	}
}

// TestGenerationPublishesRevisionAndReplays covers the happy path and the replay:
// a successful session publishes exactly one revision, promotes it, and a
// second start with unchanged draft, members and input replays it instead of
// planning again.
func TestGenerationPublishesRevisionAndReplays(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	ws := f.createCurrent("生成", ids...)

	gen := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	if gen.Status != "completed" || gen.RevisionID == "" {
		t.Fatalf("generation = %+v", gen)
	}

	// The persisted snapshot round-trips: one plan with its single step, the
	// root's live inventory facts, and the component outcome.
	detail, detailErr := f.repo.GetPlanDetail(gen.RevisionID)
	if detailErr != nil {
		t.Fatalf("GetPlanDetail: %v", detailErr)
	}
	if len(detail.Steps) != 1 || len(detail.Roots) != 1 || len(detail.Components) != 1 {
		t.Fatalf("detail steps=%d roots=%d components=%d", len(detail.Steps), len(detail.Roots), len(detail.Components))
	}
	if detail.Roots[0].EntryCount != 1 || detail.Roots[0].InventoryFingerprint == "" {
		t.Fatalf("persisted root facts = %+v", detail.Roots[0])
	}
	if detail.Steps[0].ClassifierTags == "" {
		t.Fatal("classifier tag snapshot must be persisted")
	}

	op := f.operation(ws.WorksetID)
	if op.CurrentRevision == nil || op.CurrentRevision.PlanID != gen.RevisionID {
		t.Fatalf("current revision = %+v", op.CurrentRevision)
	}
	if op.PlanningState != workset.PlanningPlanned {
		t.Fatalf("planning state = %q", op.PlanningState)
	}
	if op.CurrentRevision.Counts.Members != 1 {
		t.Fatalf("counts = %+v", op.CurrentRevision.Counts)
	}
	f.assertStoredSummary(op.CurrentRevision.PlanID)

	// Same draft, same members, same live inventory: reuse the current version.
	replay, err := f.svc.StartGeneration(
		f.ctx,
		ws.WorksetID,
		workset.OperationTypeConversion,
		workset.StartGenerationRequest{
			IfMatchVersion: op.Version, IdempotencyKey: "gen-replay",
		},
	)
	if err != nil {
		t.Fatalf("replay StartGeneration: %v", err)
	}
	if replay.Created || replay.Revision == nil || replay.Revision.PlanID != gen.RevisionID {
		t.Fatalf("expected a reuse of the current revision, got %+v", replay)
	}

	// A real input change still requires planning.
	f.insertAudioEntry("/music/albumA/02.mp3", "/music/albumA", 2048, 1001)
	fresh, err := f.svc.StartGeneration(
		f.ctx,
		ws.WorksetID,
		workset.OperationTypeConversion,
		workset.StartGenerationRequest{
			IfMatchVersion: op.Version, IdempotencyKey: "gen-after-change",
		},
	)
	if err != nil {
		t.Fatalf("StartGeneration after input change: %v", err)
	}
	if !fresh.Created {
		t.Fatalf("changed input must plan again, got %+v", fresh)
	}
}

// TestDraftChangeMakesOperationNeedPlanning covers the transition: after a draft change
// the operation is dirty, and the next successful session publishes a new
// revision.
func TestDraftChangeMakesOperationNeedPlanning(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	ws := f.createCurrent("脏检查", ids...)

	first := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	if first.Status != "completed" {
		t.Fatalf("first generation: %+v", first)
	}
	op := f.operation(ws.WorksetID)

	doc := draftDoc()
	doc.NoEffectTags = []string{"别的标签"}
	saved := f.saveDraft(ws.WorksetID, doc, op.Version)
	if saved.PlanningState != workset.PlanningNeedsPlanning {
		t.Fatalf("planning state after draft change = %q", saved.PlanningState)
	}

	second := f.runGeneration(ws.WorksetID, saved.Version)
	if second.Status != "completed" || second.RevisionID == first.RevisionID {
		t.Fatalf("second generation = %+v", second)
	}
}

// TestGenerationFailureKeepsCurrentRevision covers the failure half: a
// failed session leaves the promoted revision untouched and is not published.
func TestGenerationFailureKeepsCurrentRevision(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	ws := f.createCurrent("失败", ids...)

	first := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	if first.Status != "completed" {
		t.Fatalf("first generation: %+v", first)
	}

	// A queued session that fails (worker-side failure recorded through the
	// repository's terminal transition) must not touch the promoted revision.
	// The draft changes first so this start creates a session instead of
	// reusing the current revision.
	doc := draftDoc()
	doc.NoEffectTags = []string{"改标签"}
	saved := f.saveDraft(ws.WorksetID, doc, f.operation(ws.WorksetID).Version)
	genID := f.startGeneration(ws.WorksetID, saved.Version)
	if err := f.repo.MarkGenerationFailed(genID, "GENERATION_FAILED", "boom"); err != nil {
		t.Fatalf("MarkGenerationFailed: %v", err)
	}
	after := f.operation(ws.WorksetID)
	if after.CurrentRevision == nil || after.CurrentRevision.PlanID != first.RevisionID {
		t.Fatalf("failed session replaced the current revision: %+v", after.CurrentRevision)
	}
	if after.Version != saved.Version {
		t.Fatalf("failed session advanced the operation version: %d -> %d", saved.Version, after.Version)
	}
	if after.LatestGeneration == nil || after.LatestGeneration.Status != "failed" {
		t.Fatalf("latest generation = %+v", after.LatestGeneration)
	}
}

// TestCancelQueuedSessionIsNotPublished covers cooperative cancel: a canceled
// queued session never publishes.
func TestCancelQueuedSessionIsNotPublished(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	ws := f.createCurrent("取消", ids...)
	op := f.operation(ws.WorksetID)
	genID := f.startGeneration(ws.WorksetID, op.Version)

	canceled, err := f.svc.CancelGeneration(f.ctx, ws.WorksetID, workset.OperationTypeConversion, genID)
	if err != nil {
		t.Fatalf("CancelGeneration: %v", err)
	}
	if canceled.Status != "canceled" {
		t.Fatalf("status = %q, want canceled", canceled.Status)
	}
	after := f.operation(ws.WorksetID)
	if after.CurrentRevision != nil {
		t.Fatalf("canceled session published a revision: %+v", after.CurrentRevision)
	}
	if after.PlanningState != workset.PlanningUnplanned {
		t.Fatalf("planning state = %q", after.PlanningState)
	}
}

// TestSingleActiveSessionPerOperation pins one active session per operation.
func TestSingleActiveSessionPerOperation(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	ws := f.createCurrent("单会话", ids...)
	op := f.operation(ws.WorksetID)
	f.startGeneration(ws.WorksetID, op.Version)

	if _, err := f.svc.StartGeneration(
		f.ctx,
		ws.WorksetID,
		workset.OperationTypeConversion,
		workset.StartGenerationRequest{
			IfMatchVersion: op.Version, IdempotencyKey: "gen-second",
		},
	); err == nil {
		t.Fatal("second concurrent session must be refused")
	} else if werr, ok := workset.AsError(
		err,
	); !ok ||
		werr.Code != "GENERATION_IN_PROGRESS" {
		t.Fatalf("want GENERATION_IN_PROGRESS, got %v", err)
	}
}

// TestGenerationRejectsScanInProgress covers the library-scan gate.
func TestGenerationRejectsScanInProgress(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	ws := f.createCurrent("扫描中", ids...)
	f.exec(`
		INSERT INTO scan_sessions (session_id, root_path, kind, status, started_at)
		VALUES ('scan-1', '/music', 'full', 'running', ?)
	`, time.Now().Format(timeFmt))

	op := f.operation(ws.WorksetID)
	_, err := f.svc.StartGeneration(
		f.ctx,
		ws.WorksetID,
		workset.OperationTypeConversion,
		workset.StartGenerationRequest{
			IfMatchVersion: op.Version, IdempotencyKey: "gen-scanning",
		},
	)
	if err == nil {
		t.Fatal("generation during a scan must fail")
	}
	if werr, ok := workset.AsError(err); !ok || werr.Code != "SCAN_IN_PROGRESS" {
		t.Fatalf("want SCAN_IN_PROGRESS, got %v", err)
	}
}

// TestGenerationScopedAddressing covers the scoping rule: sessions and revisions
// are addressable only through their owning operation.
func TestGenerationScopedAddressing(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	ws := f.createCurrent("作用域", ids...)
	// A second record of another library, for the cross-record addressing
	// checks: one library holds one conversion record at a time.
	f.insertLibrary("lib-2", "/music2")
	f.insertDir("/music2", "albumC", true)
	other := f.createCurrentFor("lib-2", "另一个", "albumC")

	gen := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	if gen.Status != "completed" {
		t.Fatalf("generation: %+v", gen)
	}

	if _, err := f.svc.GetGeneration(
		f.ctx,
		other.WorksetID,
		workset.OperationTypeConversion,
		gen.GenerationID,
	); err == nil {
		t.Fatal("a session must not resolve through another workset")
	} else if werr, ok := workset.AsError(
		err,
	); !ok ||
		werr.Code != "GENERATION_NOT_FOUND" {
		t.Fatalf("want GENERATION_NOT_FOUND, got %v", err)
	}
	if _, err := f.svc.GetRevision(
		f.ctx,
		other.WorksetID,
		workset.OperationTypeConversion,
		gen.RevisionID,
	); err == nil {
		t.Fatal("a revision must not resolve through another workset")
	}
	if _, err := f.svc.GetGeneration(f.ctx, ws.WorksetID, "rename", gen.GenerationID); err == nil {
		t.Fatal("an unknown operation type must not resolve")
	}
}

// TestGenerationIdempotencyKeyReuseIsScoped covers the session idempotency
// contract: replaying the same key returns the session, a different request
// under the same key conflicts.
func TestGenerationIdempotencyKeyReuseIsScoped(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	ws := f.createCurrent("幂等", ids...)
	op := f.operation(ws.WorksetID)

	first, err := f.svc.StartGeneration(
		f.ctx,
		ws.WorksetID,
		workset.OperationTypeConversion,
		workset.StartGenerationRequest{
			IfMatchVersion: op.Version, IdempotencyKey: "same-key",
		},
	)
	if err != nil {
		t.Fatalf("StartGeneration: %v", err)
	}
	replay, err := f.svc.StartGeneration(
		f.ctx,
		ws.WorksetID,
		workset.OperationTypeConversion,
		workset.StartGenerationRequest{
			IfMatchVersion: op.Version, IdempotencyKey: "same-key",
		},
	)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Created || replay.Generation.GenerationID != first.Generation.GenerationID {
		t.Fatalf("idempotent replay returned a new session: %+v", replay)
	}
	if _, err := f.svc.StartGeneration(
		f.ctx,
		ws.WorksetID,
		workset.OperationTypeConversion,
		workset.StartGenerationRequest{
			IfMatchVersion: op.Version,
		},
	); err == nil {
		t.Fatal("a missing idempotency key must be refused")
	}
}

func TestGenerationReplayPrefersLiveKeyOverOlderTerminalRowAndCurrentPlan(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	ws := f.createCurrent("幂等回放", ids...)
	version := f.operation(ws.WorksetID).Version
	start := func(key string, match int) (*workset.StartGenerationResult, error) {
		return f.svc.StartGeneration(f.ctx, ws.WorksetID, workset.OperationTypeConversion,
			workset.StartGenerationRequest{IfMatchVersion: match, IdempotencyKey: key})
	}
	old, err := start("reused-key", version)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.repo.MarkGenerationFailed(old.Generation.GenerationID, "FAILED", "first attempt"); err != nil {
		t.Fatal(err)
	}
	live, err := start("reused-key", version)
	if err != nil {
		t.Fatal(err)
	}
	if !live.Created || live.Generation.GenerationID == old.Generation.GenerationID {
		t.Fatalf("new session = %+v", live)
	}
	replay, err := start("reused-key", version)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Created || replay.Generation == nil || replay.Generation.GenerationID != live.Generation.GenerationID {
		t.Fatalf("live replay = %+v", replay)
	}
}

func TestCompletedGenerationKeyWinsOverCurrentPlanAndStaleVersion(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	ws := f.createCurrent("已发布回放", ids...)
	before := f.operation(ws.WorksetID).Version
	completed := f.runGeneration(ws.WorksetID, before)
	if completed.Status != workset.GenStatusCompleted {
		t.Fatalf("completed = %+v", completed)
	}
	row, err := f.repo.GetGeneration(completed.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.svc.StartGeneration(f.ctx, ws.WorksetID, workset.OperationTypeConversion,
		workset.StartGenerationRequest{IfMatchVersion: before, IdempotencyKey: row.IdempotencyKey})
	if err != nil {
		t.Fatal(err)
	}
	if replay.Created || replay.Generation == nil || replay.Generation.GenerationID != completed.GenerationID {
		t.Fatalf("published session replay = %+v", replay)
	}
	doc := draftDoc()
	doc.NoEffectTags = []string{"different"}
	f.saveDraft(ws.WorksetID, doc, f.operation(ws.WorksetID).Version)
	_, err = f.svc.StartGeneration(f.ctx, ws.WorksetID, workset.OperationTypeConversion,
		workset.StartGenerationRequest{IfMatchVersion: before, IdempotencyKey: row.IdempotencyKey})
	if werr, ok := workset.AsError(err); !ok || werr.Code != "IDEMPOTENCY_KEY_REUSED" {
		t.Fatalf("changed request under completed key = %v", err)
	}
}

func TestReplacedPlanStillReplaysCompletedGeneration(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music/albumA", 1024, 1000)
	ws := f.createCurrent("替换后回放", ids...)
	before := f.operation(ws.WorksetID).Version
	first := f.runGeneration(ws.WorksetID, before)
	row, err := f.repo.GetGeneration(first.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	f.insertAudioEntry("/music/albumA/02.mp3", "/music/albumA", 2048, 1001)
	second := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	if second.RevisionID == first.RevisionID {
		t.Fatalf("plan was not replaced: %+v", second)
	}
	replay, err := f.svc.StartGeneration(f.ctx, ws.WorksetID, workset.OperationTypeConversion,
		workset.StartGenerationRequest{IfMatchVersion: before, IdempotencyKey: row.IdempotencyKey})
	if err != nil {
		t.Fatal(err)
	}
	if replay.Created || replay.Generation == nil ||
		replay.Generation.GenerationID != first.GenerationID || replay.Generation.RevisionID != "" {
		t.Fatalf("replaced plan session replay = %+v", replay)
	}
}
