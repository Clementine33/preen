package sqlite_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/library"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func newExecutionRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

const execDraftHash = "draft-hash"

// seedExecutionWorkset writes a workset with a current revision and its draft,
// so the guarded execution insert has real facts to check.
func seedExecutionWorkset(t *testing.T, repo *sqlite.Repository, worksetID, root string) {
	t.Helper()
	now := time.Now().Format(time.RFC3339Nano)
	if _, err := repo.DB().Exec(`
		INSERT INTO libraries (id, name, root_path, root_path_key, created_at, updated_at)
		VALUES ('lib-1', 'Onsei', ?, ?, ?, ?)
	`, root, root, now, now); err != nil {
		t.Fatalf("seed library: %v", err)
	}
	if _, err := repo.DB().Exec(`
		INSERT INTO worksets (id, title, library_id, root_path, root_path_key, version, created_at, updated_at)
		VALUES (?, 'exec ws', 'lib-1', ?, ?, 1, ?, ?)
	`, worksetID, root, root, now, now); err != nil {
		t.Fatalf("seed workset: %v", err)
	}
	if _, err := repo.DB().Exec(`
		INSERT INTO workset_operations (workset_id, operation_type, version, current_revision_id, created_at, updated_at)
		VALUES (?, 'conversion', 2, 'plan-1', ?, ?)
	`, worksetID, now, now); err != nil {
		t.Fatalf("seed operation: %v", err)
	}
	if _, err := repo.DB().Exec(`
		INSERT INTO workset_operation_drafts (workset_id, operation_type, schema_version, draft_json, draft_hash, updated_at)
		VALUES (?, 'conversion', 1, '{}', ?, ?)
	`, worksetID, execDraftHash, now); err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	if _, err := repo.DB().Exec(`
		INSERT INTO plans (plan_id, root_path, snapshot_token, task_kind, task_schema_version, workset_id, created_at)
		VALUES ('plan-1', ?, 'snap', 'conversion', 1, ?, ?)
	`, root, worksetID, now); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
}

// insertExecutionRow writes one session row directly.
func insertExecutionRow(t *testing.T, repo *sqlite.Repository, e *workset.PlanExecution) {
	t.Helper()
	now := time.Now().Format(time.RFC3339Nano)
	if _, err := repo.DB().Exec(`
		INSERT INTO plan_executions (
			execution_id, workset_id, operation_type, plan_id, status,
			idempotency_key, request_hash, expected_operation_version,
			total_components, total_operations, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, 'queued', ?, ?, 0, 0, 0, ?, ?)
	`, e.ExecutionID, e.WorksetID, e.OperationType, e.PlanID,
		e.IdempotencyKey, e.RequestHash, now, now); err != nil {
		t.Fatalf("insert execution row %s: %v", e.ExecutionID, err)
	}
	if _, err := repo.DB().Exec(
		`INSERT INTO plan_execution_requests(execution_id, request_json) VALUES (?, ?)`,
		e.ExecutionID, e.RequestJSON,
	); err != nil {
		t.Fatalf("insert execution request %s: %v", e.ExecutionID, err)
	}
}

// createExecution inserts a queued session for the seeded revision.
func createExecution(t *testing.T, repo *sqlite.Repository, e *workset.PlanExecution, planID string) error {
	t.Helper()
	return repo.CreateExecutionGuarded(e, workset.ExecutionGuards{
		ExpectedOperationVersion: 2,
		ExpectedCurrentRevision:  planID,
		ExpectedDraftHash:        execDraftHash,
	})
}

// newExecution builds a minimal queued session row.
func newExecution(id, worksetID, planID, key string) *workset.PlanExecution {
	return &workset.PlanExecution{
		ExecutionID:     id,
		WorksetID:       worksetID,
		OperationType:   "conversion",
		PlanID:          planID,
		IdempotencyKey:  key,
		RequestHash:     "hash-" + key,
		RequestJSON:     `{"delete_mode":"soft","units":[]}`,
		TotalComponents: 0,
		CreatedAt:       time.Now(),
	}
}

func TestPlanExecutionClaimCancelAndTerminalGuard(t *testing.T) {
	repo := newExecutionRepo(t)
	seedExecutionWorkset(t, repo, "ws-1", "/music")
	if err := createExecution(t, repo, newExecution("exec-1", "ws-1", "plan-1", "key-1"), "plan-1"); err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}

	claimed, err := repo.NextQueuedExecution()
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	if claimed.Status != workset.ExecStatusRunning || claimed.StartedAt.IsZero() {
		t.Fatalf("claimed row = %+v", claimed)
	}
	// The queue is drained: the same claim returns nothing.
	if again, claimErr := repo.NextQueuedExecution(); claimErr != nil || again != nil {
		t.Fatalf("second claim must be empty: %v %+v", claimErr, again)
	}

	// Cancel of a running session sets the cooperative flag only.
	if cancelErr := repo.CancelExecution("exec-1"); cancelErr != nil {
		t.Fatalf("CancelExecution: %v", cancelErr)
	}
	running, err := repo.GetExecution("exec-1")
	if err != nil {
		t.Fatalf("GetExecution: %v", err)
	}
	if running.Status != workset.ExecStatusRunning || !running.CancelRequested {
		t.Fatalf("running row after cancel = %+v", running)
	}

	// Terminal write, then a late worker write must not downgrade it.
	if finishErr := repo.FinishExecution("exec-1", workset.ExecStatusSucceeded, "", ""); finishErr != nil {
		t.Fatalf("FinishExecution: %v", finishErr)
	}
	if finishErr := repo.FinishExecution("exec-1", workset.ExecStatusFailed, "X", "late"); finishErr != nil {
		t.Fatalf("late FinishExecution: %v", finishErr)
	}
	done, err := repo.GetExecution("exec-1")
	if err != nil {
		t.Fatalf("GetExecution: %v", err)
	}
	if done.Status != workset.ExecStatusSucceeded || done.FinishedAt.IsZero() {
		t.Fatalf("terminal row = %+v", done)
	}
	if active, err := repo.GetActiveExecutionForOperation("ws-1", "conversion"); err != nil || active != nil {
		t.Fatalf("terminal session must not be active: %v %+v", err, active)
	}
}

func TestCreateExecutionGuardedRejections(t *testing.T) {
	guards := workset.ExecutionGuards{
		ExpectedOperationVersion: 2,
		ExpectedCurrentRevision:  "plan-1",
		ExpectedDraftHash:        execDraftHash,
	}
	t.Run("stale_version", func(t *testing.T) {
		repo := newExecutionRepo(t)
		seedExecutionWorkset(t, repo, "ws-1", "/music")
		stale := guards
		stale.ExpectedOperationVersion = 1
		err := repo.CreateExecutionGuarded(newExecution("exec-1", "ws-1", "plan-1", "key-1"), stale)
		if !errors.Is(err, workset.ErrVersionConflict) {
			t.Fatalf("err = %v, want workset.ErrVersionConflict", err)
		}
	})
	t.Run("draft_drift", func(t *testing.T) {
		repo := newExecutionRepo(t)
		seedExecutionWorkset(t, repo, "ws-1", "/music")
		if _, err := repo.DB().Exec(
			"UPDATE workset_operation_drafts SET draft_hash = 'moved-on' WHERE workset_id = 'ws-1'",
		); err != nil {
			t.Fatalf("drift draft: %v", err)
		}
		err := repo.CreateExecutionGuarded(newExecution("exec-1", "ws-1", "plan-1", "key-1"), guards)
		if !errors.Is(err, workset.ErrDraftChanged) {
			t.Fatalf("err = %v, want ErrDraftChanged", err)
		}
	})
	t.Run("not_current_revision", func(t *testing.T) {
		repo := newExecutionRepo(t)
		seedExecutionWorkset(t, repo, "ws-1", "/music")
		if _, err := repo.DB().Exec(
			"UPDATE workset_operations SET current_revision_id = 'plan-other' WHERE workset_id = 'ws-1'",
		); err != nil {
			t.Fatalf("promote other revision: %v", err)
		}
		err := repo.CreateExecutionGuarded(newExecution("exec-1", "ws-1", "plan-1", "key-1"), guards)
		if !errors.Is(err, workset.ErrRevisionNotFound) {
			t.Fatalf("err = %v, want workset.ErrRevisionNotFound", err)
		}
	})
	t.Run("orphaned_workset", func(t *testing.T) {
		repo := newExecutionRepo(t)
		seedExecutionWorkset(t, repo, "ws-1", "/music")
		if _, err := repo.DB().Exec("UPDATE worksets SET library_id = NULL WHERE id = 'ws-1'"); err != nil {
			t.Fatalf("orphan workset: %v", err)
		}
		err := repo.CreateExecutionGuarded(newExecution("exec-1", "ws-1", "plan-1", "key-1"), guards)
		if !errors.Is(err, workset.ErrWorksetOrphaned) {
			t.Fatalf("err = %v, want ErrWorksetOrphaned", err)
		}
	})
	t.Run("generation_in_progress", func(t *testing.T) {
		repo := newExecutionRepo(t)
		seedExecutionWorkset(t, repo, "ws-1", "/music")
		if _, err := repo.DB().Exec(`
			INSERT INTO plan_generations (generation_id, workset_id, operation_type, status, created_at, updated_at)
			VALUES ('gen-1', 'ws-1', 'conversion', 'running', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`); err != nil {
			t.Fatalf("seed generation: %v", err)
		}
		err := repo.CreateExecutionGuarded(newExecution("exec-1", "ws-1", "plan-1", "key-1"), guards)
		if !errors.Is(err, library.ErrGenerationInProgress) {
			t.Fatalf("err = %v, want library.ErrGenerationInProgress", err)
		}
	})
}

func TestPlanExecutionCancelQueuedSkipsTheWorker(t *testing.T) {
	repo := newExecutionRepo(t)
	seedExecutionWorkset(t, repo, "ws-1", "/music")
	if err := createExecution(t, repo, newExecution("exec-1", "ws-1", "plan-1", "key-1"), "plan-1"); err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	if cancelErr := repo.CancelExecution("exec-1"); cancelErr != nil {
		t.Fatalf("CancelExecution: %v", cancelErr)
	}
	canceled, err := repo.GetExecution("exec-1")
	if err != nil {
		t.Fatalf("GetExecution: %v", err)
	}
	if canceled.Status != workset.ExecStatusCanceled || canceled.FinishedAt.IsZero() {
		t.Fatalf("canceled row = %+v", canceled)
	}
	if claimed, err := repo.NextQueuedExecution(); err != nil || claimed != nil {
		t.Fatalf("canceled-queued session must never be claimed: %v %+v", err, claimed)
	}
}

func TestPlanExecutionInterruptStaleKeepsPartialResults(t *testing.T) {
	repo := newExecutionRepo(t)
	seedExecutionWorkset(t, repo, "ws-1", "/music")
	queued := newExecution("exec-q", "ws-1", "plan-1", "key-q")
	running := newExecution("exec-r", "ws-1", "plan-2", "key-r")
	if _, err := repo.DB().Exec(`INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id)
		VALUES('plan-2','/music','snap','ws-1')`); err != nil {
		t.Fatalf("seed second plan: %v", err)
	}
	// Rows are written directly: this scenario exercises the startup
	// interruption of leftovers, not the guarded creation path.
	insertExecutionRow(t, repo, queued)
	insertExecutionRow(t, repo, running)
	if _, err := repo.NextQueuedExecution(); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := repo.SaveExecutionComponentResult("exec-r", workset.ExecutionComponentResult{
		ComponentIndex:      0,
		Status:              "succeeded",
		CompletedOperations: 2,
		ResultJSON:          `{"committed":["/music/a/01.mp3"]}`,
	}, workset.ExecutionPosition{CurrentComponentID: "u1", CurrentComponentIndex: 1}); err != nil {
		t.Fatalf("save component result: %v", err)
	}

	n, err := repo.InterruptStaleExecutions()
	if err != nil {
		t.Fatalf("InterruptStaleExecutions: %v", err)
	}
	if n != 2 {
		t.Fatalf("interrupted = %d, want 2", n)
	}
	for _, id := range []string{"exec-q", "exec-r"} {
		row, loadErr := repo.GetExecution(id)
		if loadErr != nil {
			t.Fatalf("GetExecution %s: %v", id, loadErr)
		}
		if row.Status != workset.ExecStatusInterrupted {
			t.Fatalf("%s status = %s, want interrupted", id, row.Status)
		}
	}
	kept, err := repo.GetExecution("exec-r")
	if err != nil {
		t.Fatalf("GetExecution: %v", err)
	}
	if kept.CompletedComponents != 1 || kept.CompletedOperations != 2 {
		t.Fatalf("interrupted session lost its counters: %+v", kept)
	}
	results, err := repo.ListExecutionComponentResults("exec-r", 0, 0)
	if err != nil {
		t.Fatalf("ListExecutionComponentResults: %v", err)
	}
	if len(results) != 1 || results[0].ResultJSON != `{"committed":["/music/a/01.mp3"]}` {
		t.Fatalf("interrupted session lost its component results: %+v", results)
	}
}

func TestPlanExecutionIdempotencyKeyIsHeldForTheSessionLife(t *testing.T) {
	repo := newExecutionRepo(t)
	seedExecutionWorkset(t, repo, "ws-1", "/music")
	if err := createExecution(t, repo, newExecution("exec-1", "ws-1", "plan-1", "key-1"), "plan-1"); err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	// Same key, same operation: replayed row.
	last := newExecution("exec-2", "ws-1", "plan-1", "key-1")
	last.RequestHash = "hash-key-1"
	if err := createExecution(t, repo, last, "plan-1"); !errors.Is(err, workset.ErrExecutionIdemConflict) {
		t.Fatalf("duplicate key err = %v, want ErrExecutionIdemConflict", err)
	}
	existing, err := repo.GetExecutionByOperationKey("ws-1", "conversion", "key-1")
	if err != nil || existing == nil || existing.ExecutionID != "exec-1" {
		t.Fatalf("replay lookup = %v %+v", err, existing)
	}
	if unknown, err := repo.GetExecutionByOperationKey("ws-1", "conversion", "nope"); err != nil || unknown != nil {
		t.Fatalf("unknown key = %v %+v", err, unknown)
	}
}

func TestPlanExecutionOneRowPerRevision(t *testing.T) {
	repo := newExecutionRepo(t)
	seedExecutionWorkset(t, repo, "ws-1", "/music")
	if err := createExecution(t, repo, newExecution("exec-1", "ws-1", "plan-1", "key-1"), "plan-1"); err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	// A second session for the same revision is a storage-level conflict, not a
	// check the callers must remember.
	second := newExecution("exec-2", "ws-1", "plan-1", "key-2")
	if err := createExecution(t, repo, second, "plan-1"); !errors.Is(err, workset.ErrExecutionIdemConflict) {
		t.Fatalf("second session err = %v, want ErrExecutionIdemConflict", err)
	}
	if existing, err := repo.GetExecutionForRevision(
		"plan-1",
	); err != nil || existing == nil ||
		existing.ExecutionID != "exec-1" {
		t.Fatalf("revision lookup = %v %+v", err, existing)
	}
	if none, err := repo.GetExecutionForRevision("plan-none"); err != nil || none != nil {
		t.Fatalf("unexecuted revision = %v %+v", err, none)
	}
}

func TestPlanExecutionActiveForRootGuard(t *testing.T) {
	repo := newExecutionRepo(t)
	seedExecutionWorkset(t, repo, "ws-1", "/music")
	if err := createExecution(t, repo, newExecution("exec-1", "ws-1", "plan-1", "key-1"), "plan-1"); err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	active, err := repo.HasActiveExecutionForRoot("/music")
	if err != nil || !active {
		t.Fatalf("active guard = %v %v, want true", active, err)
	}
	if other, guardErr := repo.HasActiveExecutionForRoot("/elsewhere"); guardErr != nil || other {
		t.Fatalf("unrelated root guard = %v %v, want false", other, guardErr)
	}
	if finishErr := repo.FinishExecution("exec-1", workset.ExecStatusSucceeded, "", ""); finishErr != nil {
		t.Fatalf("FinishExecution: %v", finishErr)
	}
	active, err = repo.HasActiveExecutionForRoot("/music")
	if err != nil || active {
		t.Fatalf("guard after terminal = %v %v, want false", active, err)
	}
}

func TestSyncObservedInventoryLifecycle(t *testing.T) {
	repo := newExecutionRepo(t)
	now := time.Now().Format(time.RFC3339Nano)
	seed := func(path string, size, mtime, contentRev int64, bitrate *int32) {
		t.Helper()
		inventoryfixture.SeedInventory(
			t,
			repo,
			inventoryfixture.Row{
				Path:       path,
				RootPath:   "/music",
				Size:       size,
				Mtime:      mtime,
				ContentRev: int(contentRev),
				Bitrate:    bitrate,
			},
		)
	}
	seed("/music/album/changed.mp3", 100, 1000, 3, new(int32(5000)))
	seed("/music/album/gone.mp3", 100, 1000, 1, nil)
	if _, err := repo.DB().Exec(`
		INSERT INTO generation_records
			(path, codec, encoder, encoder_version, bitrate_kbps, mode, size, mtime, content_sha256, created_at)
		VALUES ('/music/album/gone.mp3', 'mp3', 'libmp3lame', 'ffmpeg version n9.0.1', 320, 'cbr', 100, 1000, 'stale', ?)
	`, now); err != nil {
		t.Fatalf("seed generation record: %v", err)
	}

	err := repo.SyncObservedInventory("/music",
		[]string{"/music/album/gone.mp3"},
		[]inventory.InventoryFile{
			{Path: "/music/album/changed.mp3", Size: 222, Mtime: 2000},
			{Path: "/music/album/Delete/nested/gone.mp3", Size: 100, Mtime: 1000},
		},
		[]inventory.GenerationRecord{{
			Path: "/music/album/changed.mp3", Codec: "mp3", Encoder: "libmp3lame",
			EncoderVersion: "ffmpeg version n9.0.1", BitrateKbps: 320, Mode: "cbr",
			Size: 222, Mtime: 2000, ContentSHA256: "0f1e2d",
		}},
	)
	if err != nil {
		t.Fatalf("SyncObservedInventory: %v", err)
	}

	changed := inventoryEntry(t, repo, "/music/album/changed.mp3")
	if changed.ContentRev != 4 || changed.Size != 222 || changed.Mtime != 2000 || changed.Bitrate != nil {
		t.Fatalf("changed row=%+v", changed)
	}

	var n int
	{
		found, readErr := repo.RootExistsInInventory("/music/album/gone.mp3")
		if readErr != nil {
			t.Fatal(readErr)
		}
		n = 0
		if found {
			n = 1
		}
	}
	if n != 0 {
		t.Fatal("removed source row must be gone")
	}
	for _, p := range []string{"/music", "/music/album", "/music/album/Delete", "/music/album/Delete/nested"} {
		found, readErr := repo.RootExistsInInventory(p)
		if readErr != nil || !found {
			t.Fatalf("output ancestor %s: %v, %v", p, found, readErr)
		}
	}
	assertGenerationRecords(t, repo)

	var parent, name string
	var revNew int64
	{
		entry := inventoryEntry(t, repo, "/music/album/Delete/nested/gone.mp3")
		parent = entry.ParentPath
		name = entry.Name
		revNew = int64(entry.ContentRev)
	}
	if parent != "/music/album/Delete/nested" || name != "gone.mp3" || revNew != 1 {
		t.Fatalf("recovery row = parent=%q name=%q rev=%d", parent, name, revNew)
	}

	// An unchanged observation keeps a newly measured bitrate and the revision.
	if err = repo.UpdateEntryBitrates(
		[]inventory.BitrateUpdate{{Path: "/music/album/changed.mp3", BitrateKbps: 320000}},
		false,
	); err != nil {
		t.Fatal(err)
	}
	err = repo.SyncObservedInventory("/music", nil, []inventory.InventoryFile{
		{Path: "/music/album/changed.mp3", Size: 222, Mtime: 2000},
	}, nil)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	unchanged := inventoryEntry(t, repo, "/music/album/changed.mp3")
	if unchanged.ContentRev != 4 || unchanged.Bitrate == nil || *unchanged.Bitrate != 320000 {
		t.Fatalf("unchanged facts=%+v", unchanged)
	}
}

// assertGenerationRecords checks how a credential follows the inventory: a
// removed path loses its record (it described bytes that are no longer there,
// and a later arrival at that path must not inherit it), and a committed
// output's record lands beside its refreshed row carrying the encoder that
// wrote it and the hash of those bytes.
func assertGenerationRecords(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	var n int
	if err := repo.DB().QueryRow(
		"SELECT COUNT(*) FROM generation_records WHERE path = '/music/album/gone.mp3'",
	).Scan(&n); err != nil {
		t.Fatalf("removed record: %v", err)
	}
	if n != 0 {
		t.Fatal("the credential of a removed path must go with it")
	}

	var codec, encoder, mode, digest string
	var kbps, recordSize, recordMtime int64
	if err := repo.DB().QueryRow(`
		SELECT codec, encoder, bitrate_kbps, mode, size, mtime, content_sha256
		FROM generation_records WHERE path = '/music/album/changed.mp3'
	`).Scan(&codec, &encoder, &kbps, &mode, &recordSize, &recordMtime, &digest); err != nil {
		t.Fatalf("generation record: %v", err)
	}
	if codec != "mp3" || encoder != "libmp3lame" || kbps != 320 || mode != "cbr" ||
		recordSize != 222 || recordMtime != 2000 || digest != "0f1e2d" {
		t.Fatalf(
			"record = %s/%s %dk %s size=%d mtime=%d %s",
			codec, encoder, kbps, mode, recordSize, recordMtime, digest,
		)
	}
}

// TestSaveExecutionComponentResultIsReplaySafe pins the two properties the
// boundary write leans on: a result asked for twice is stored once, and the
// session's counters move by the difference from the stored row, so a replay
// after an uncertain commit cannot double-count.
func TestSaveExecutionComponentResultIsReplaySafe(t *testing.T) {
	repo := newExecutionRepo(t)
	seedExecutionWorkset(t, repo, "ws-1", "/music")
	if err := createExecution(t, repo, newExecution("exec-1", "ws-1", "plan-1", "key-1"), "plan-1"); err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}

	first := workset.ExecutionComponentResult{
		ComponentIndex:      0,
		Status:              "succeeded",
		CompletedOperations: 3,
		ResultJSON:          `{"committed":["/music/a/01.mp3"]}`,
	}
	progress := workset.ExecutionPosition{CurrentComponentIndex: 1}
	if err := repo.SaveExecutionComponentResult("exec-1", first, progress); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// The same write again, as a retry after an uncertain commit would send it.
	if replayErr := repo.SaveExecutionComponentResult("exec-1", first, progress); replayErr != nil {
		t.Fatalf("replayed save: %v", replayErr)
	}

	row, err := repo.GetExecution("exec-1")
	if err != nil {
		t.Fatalf("GetExecution: %v", err)
	}
	if row.CompletedComponents != 1 || row.CompletedOperations != 3 {
		t.Fatalf("replayed write double-counted: %+v", row)
	}
	results, err := repo.ListExecutionComponentResults("exec-1", 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("replayed write duplicated the row: %+v", results)
	}

	// A component that is still pending neither counts nor hides its facts.
	if pendingErr := repo.SaveExecutionComponentResult("exec-1", workset.ExecutionComponentResult{
		ComponentIndex: 1,
		Status:         "pending",
		ResultJSON:     `{"remaining":["/music/b/01.mp3"]}`,
	}, workset.ExecutionPosition{}); pendingErr != nil {
		t.Fatalf("pending save: %v", pendingErr)
	}
	row, err = repo.GetExecution("exec-1")
	if err != nil {
		t.Fatalf("GetExecution: %v", err)
	}
	if row.CompletedComponents != 1 {
		t.Fatalf("a pending component counted as completed: %+v", row)
	}
	results, err = repo.ListExecutionComponentResults("exec-1", 1, 0)
	if err != nil {
		t.Fatalf("list from 1: %v", err)
	}
	if len(results) != 1 || results[0].ResultJSON != `{"remaining":["/music/b/01.mp3"]}` {
		t.Fatalf("pending facts were not kept: %+v", results)
	}
	if page, err := repo.ListExecutionComponentResults("exec-1", 1, 1); err != nil || len(page) != 1 {
		t.Fatalf("limited page = %+v err=%v", page, err)
	}
}

// TestSaveExecutionComponentResultCountersMatchRecount pins the incremental
// counters against a fresh COUNT/SUM over the stored rows after first saves,
// identical repeats and overwrites that change status and operations.
func TestSaveExecutionComponentResultCountersMatchRecount(t *testing.T) {
	repo := newExecutionRepo(t)
	seedExecutionWorkset(t, repo, "ws-1", "/music")
	if err := createExecution(t, repo, newExecution("exec-1", "ws-1", "plan-1", "key-1"), "plan-1"); err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	for _, step := range []struct {
		name       string
		index      int
		status     string
		operations int
		components int
		total      int
	}{
		{"first save", 0, "succeeded", 3, 1, 3},
		{"identical repeat", 0, "succeeded", 3, 1, 3},
		{"new pending", 1, "pending", 2, 1, 5},
		{"pending to done with other ops", 1, "succeeded", 4, 2, 7},
		{"ops change only", 0, "succeeded", 1, 2, 5},
		{"done back to pending", 0, "pending", 0, 1, 4},
	} {
		if err := repo.SaveExecutionComponentResult("exec-1", workset.ExecutionComponentResult{
			ComponentIndex:      step.index,
			Status:              step.status,
			CompletedOperations: step.operations,
			ResultJSON:          `{}`,
		}, workset.ExecutionPosition{}); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		row, err := repo.GetExecution("exec-1")
		if err != nil {
			t.Fatalf("%s: GetExecution: %v", step.name, err)
		}
		if row.CompletedComponents != step.components || row.CompletedOperations != step.total {
			t.Fatalf("%s: counters = %d/%d, want %d/%d", step.name,
				row.CompletedComponents, row.CompletedOperations, step.components, step.total)
		}
		var components, operations int
		if err = repo.DB().QueryRow(`SELECT COUNT(*) FILTER (WHERE status <> 'pending'),
			COALESCE(SUM(completed_operations), 0) FROM execution_component_results
			WHERE execution_id = 'exec-1'`).Scan(&components, &operations); err != nil {
			t.Fatalf("%s: recount: %v", step.name, err)
		}
		if components != row.CompletedComponents || operations != row.CompletedOperations {
			t.Fatalf("%s: counters %d/%d drifted from recount %d/%d", step.name,
				row.CompletedComponents, row.CompletedOperations, components, operations)
		}
	}
}

// TestExecutionComponentResultsGoWithTheirSession pins the cleanup path: the
// child rows are removed with the session they belong to, so a retired plan
// cannot leave them behind.
func TestExecutionComponentResultsGoWithTheirSession(t *testing.T) {
	repo := newExecutionRepo(t)
	seedExecutionWorkset(t, repo, "ws-1", "/music")
	if err := createExecution(t, repo, newExecution("exec-1", "ws-1", "plan-1", "key-1"), "plan-1"); err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	if err := repo.SaveExecutionComponentResult("exec-1", workset.ExecutionComponentResult{
		ComponentIndex: 0,
		Status:         "succeeded",
		ResultJSON:     `{}`,
	}, workset.ExecutionPosition{}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := repo.DB().Exec("DELETE FROM plan_executions WHERE plan_id = 'plan-1'"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	results, err := repo.ListExecutionComponentResults("exec-1", 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("component results outlived their session: %+v", results)
	}
}

func TestExecutionRequestIsStoredApartAndGoesWithItsSession(t *testing.T) {
	repo := newExecutionRepo(t)
	seedExecutionWorkset(t, repo, "ws-1", "/music")
	e := newExecution("exec-1", "ws-1", "plan-1", "key-1")
	e.RequestJSON = `{"units":[1,2,3]}`
	if err := createExecution(t, repo, e, "plan-1"); err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	got, err := repo.GetExecution("exec-1")
	if err != nil || got.RequestJSON != e.RequestJSON {
		t.Fatalf("GetExecution request = %q, %v", got.RequestJSON, err)
	}
	replay, err := repo.GetExecutionByOperationKey("ws-1", "conversion", "key-1")
	if err != nil || replay == nil || replay.RequestJSON != e.RequestJSON {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	if _, err = repo.DB().Exec("DELETE FROM plan_executions WHERE plan_id = 'plan-1'"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	var orphans int
	if err = repo.DB().
		QueryRow("SELECT COUNT(*) FROM plan_execution_requests").
		Scan(&orphans); err != nil ||
		orphans != 0 {
		t.Fatalf("request rows outlived their session: %d, %v", orphans, err)
	}
}

// BenchmarkExecutionComponentResults measures what one execution session's
// writes cost now that each component commits its own result: the boundary
// transaction is one result row plus the counter recomputation, and nothing is
// rewritten as the run grows. Run it with
//
//	go test ./internal/adapters/sqlite -run '^$' -bench ExecutionComponentResults
//
// The reported metrics are per session (b.N sessions) and per component: the
// point of the redesign is that the result bytes scale with the work that
// happened, and the WAL bytes with the rows written, not with the square of the
// component count.
func BenchmarkExecutionComponentResults(b *testing.B) {
	const components = 1000
	dir := b.TempDir()
	dbPath := filepath.Join(dir, "bench.db")
	repo, err := sqlite.NewRepository(dbPath)
	if err != nil {
		b.Fatalf("new repo: %v", err)
	}
	b.Cleanup(func() { _ = repo.Close() })
	seedBenchWorkset(b, repo)
	// Keep the WAL from checkpointing mid-run so its growth is what the session
	// itself wrote.
	if _, err := repo.DB().Exec("PRAGMA wal_autocheckpoint = 0"); err != nil {
		b.Fatalf("disable autocheckpoint: %v", err)
	}

	result := workset.ExecutionComponentResult{
		Status:              "succeeded",
		CompletedOperations: 2,
		ResultJSON: `{"committed":["/music/albumA/01.mp3","/music/albumA/02.mp3"],` +
			`"removed":["/music/albumA/01.wav"],"recovery":["/music/Delete/albumA/01.wav"]}`,
	}

	var walBytes, resultBytes int64
	b.ResetTimer()
	for i := range b.N {
		// One session per revision is a storage invariant, so each iteration
		// gets its own plan id.
		executionID := "exec-bench-" + strconv.Itoa(i)
		if _, err := repo.DB().Exec(`INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id)
			VALUES(?,'/music','snap','ws-bench')`, "plan-bench-"+strconv.Itoa(i)); err != nil {
			b.Fatalf("seed plan: %v", err)
		}
		if _, err := repo.DB().Exec(`
			INSERT INTO plan_executions (
				execution_id, workset_id, operation_type, plan_id, status,
				total_components, created_at, updated_at
			) VALUES (?, 'ws-bench', 'conversion', ?, 'running', ?, '', '')
		`, executionID, "plan-bench-"+strconv.Itoa(i), components); err != nil {
			b.Fatalf("seed session: %v", err)
		}
		if _, err := repo.DB().Exec(
			`INSERT INTO plan_execution_requests(execution_id, request_json) VALUES (?, '{}')`, executionID,
		); err != nil {
			b.Fatalf("seed request: %v", err)
		}
		before := fileSize(b, dbPath+"-wal")
		for c := range components {
			if err := repo.SaveExecutionComponentResult(executionID, workset.ExecutionComponentResult{
				ComponentIndex:      c,
				Status:              result.Status,
				CompletedOperations: result.CompletedOperations,
				ResultJSON:          result.ResultJSON,
			}, workset.ExecutionPosition{CurrentComponentIndex: c + 1}); err != nil {
				b.Fatalf("save component %d: %v", c, err)
			}
		}
		if err := repo.FinishExecution(executionID, workset.ExecStatusSucceeded, "", ""); err != nil {
			b.Fatalf("finish: %v", err)
		}
		walBytes += fileSize(b, dbPath+"-wal") - before
		resultBytes += int64(len(result.ResultJSON)) * components
	}
	b.StopTimer()

	b.ReportMetric(float64(walBytes)/float64(b.N)/float64(components), "WAL-bytes/component")
	b.ReportMetric(float64(resultBytes)/float64(b.N)/float64(components), "result-bytes/component")
	b.ReportMetric(float64(components), "components/session")
}

func seedBenchWorkset(b *testing.B, repo *sqlite.Repository) {
	b.Helper()
	now := time.Now().Format(time.RFC3339Nano)
	for _, stmt := range []string{
		`INSERT INTO libraries (id, name, root_path, root_path_key, created_at, updated_at)
		 VALUES ('lib-bench', 'bench', '/music', '/music', ?, ?)`,
		`INSERT INTO worksets (id, title, library_id, root_path, root_path_key, version, created_at, updated_at)
		 VALUES ('ws-bench', 'bench', 'lib-bench', '/music', '/music', 1, ?, ?)`,
		`INSERT INTO plans (plan_id, root_path, snapshot_token, task_kind, task_schema_version, workset_id, created_at)
		 VALUES ('plan-bench', '/music', 'snap', 'conversion', 1, 'ws-bench', ?)`,
	} {
		if _, err := repo.DB().Exec(stmt, now, now); err != nil {
			b.Fatalf("seed bench workset: %v", err)
		}
	}
}

func fileSize(b *testing.B, path string) int64 {
	b.Helper()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		b.Fatalf("stat %s: %v", path, err)
	}
	return info.Size()
}

func TestListExecutionResultDigestsProjectsStatusFacts(t *testing.T) {
	repo := newExecutionRepo(t)
	seedExecutionWorkset(t, repo, "ws-1", "/music")
	insertExecutionRow(t, repo, newExecution("exec-d", "ws-1", "plan-1", "key-d"))
	for i, res := range []workset.ExecutionComponentResult{
		{Status: "failed", CompletedOperations: 1, ResultJSON: `{"error_code":"E","error_message":"m","inventory_sync_error":"s","committed":["/a"]}`},
		{Status: "succeeded", ResultJSON: `{}`},
		{Status: "failed", ResultJSON: `not json`},
	} {
		res.ComponentIndex = i
		if err := repo.SaveExecutionComponentResult("exec-d", res, workset.ExecutionPosition{}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.ListExecutionResultDigests("exec-d", 1)
	if err != nil || len(got) != 2 || got[0].ComponentIndex != 1 || got[0].ErrorCode != "" || !got[1].Unreadable {
		t.Fatalf("digests from 1 = %+v, %v", got, err)
	}
	all, err := repo.ListExecutionResultDigests("exec-d", 0)
	if err != nil || len(all) != 3 {
		t.Fatalf("digests = %+v, %v", all, err)
	}
	want := workset.ExecutionResultDigest{
		Status: "failed", CompletedOperations: 1, ErrorCode: "E", ErrorMessage: "m", InventorySyncError: "s",
	}
	if all[0] != want {
		t.Fatalf("digest 0 = %+v", all[0])
	}
}
