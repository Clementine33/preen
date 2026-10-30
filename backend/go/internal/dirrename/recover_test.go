package dirrename_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/dirrename"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
	"github.com/Clementine33/preen/backend/internal/workset"
)

type renameRun struct {
	repo                         *sqlite.Repository
	task                         *dirrename.Task
	svc                          workset.Service
	gate                         *admission.Gate
	member                       *workset.WorksetMember
	worksetID, planID, libraryID string
}

func newRenameRun(t *testing.T) renameRun {
	t.Helper()
	repo, task, m := fixture(t, "title")
	gate := admission.NewGate(repo.HasActiveSession)
	svc := workset.NewService(
		repo,
		1,
		1,
		[]workset.Task{task, refresh.NewTask(repo, filesystem.CheckWorkDirectory)},
		nil,
		gate.Enqueue,
	)
	svc.SetExclusiveEnqueueGuard(gate.EnqueueExclusive)
	libs, err := repo.ListLibraries()
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateCurrentWorkset(
		t.Context(),
		workset.CreateCurrentRequest{LibraryID: libs[0].ID, OperationType: "rename", FolderPaths: []string{m.RelPath}},
	)
	if err != nil {
		t.Fatal(err)
	}
	id := created.Workset.WorksetID
	gen, err := svc.StartGeneration(
		t.Context(),
		id,
		"rename",
		workset.StartGenerationRequest{IfMatchVersion: 1, IdempotencyKey: "plan"},
	)
	if err != nil {
		t.Fatal(err)
	}
	svc.DispatcherHandle().Start()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		view, err := svc.GetGeneration(t.Context(), id, "rename", gen.Generation.GenerationID)
		if err != nil {
			t.Fatal(err)
		}
		if view.Status == workset.GenStatusCompleted {
			if stopErr := svc.DispatcherHandle().Stop(context.Background()); stopErr != nil {
				t.Error(stopErr)
			}
			return renameRun{repo, task, svc, gate, m, id, view.RevisionID, libs[0].ID}
		}
		if view.Status == workset.GenStatusFailed {
			t.Fatalf("generation: %+v", view)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if stopErr := svc.DispatcherHandle().Stop(context.Background()); stopErr != nil {
		t.Error(stopErr)
	}
	t.Fatal("generation timed out")
	return renameRun{}
}
func (f renameRun) start(t *testing.T) *workset.PlanExecution {
	t.Helper()
	op, err := f.svc.GetOperation(t.Context(), f.worksetID, "rename")
	if err != nil {
		t.Fatal(err)
	}
	started, err := f.svc.StartExecution(
		t.Context(),
		f.worksetID,
		"rename",
		f.planID,
		workset.StartExecutionRequest{IfMatchVersion: op.Version, IdempotencyKey: "execute"},
	)
	if err != nil {
		t.Fatal(err)
	}
	ex, err := f.repo.GetExecution(started.Execution.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	return ex
}
func TestStartupRepairOnlyTouchesFirstMissingResultAndPersistsIt(t *testing.T) {
	for _, synced := range []bool{false, true} {
		t.Run(map[bool]string{false: "before sync", true: "after sync"}[synced], func(t *testing.T) {
			f := newRenameRun(t)
			ex := f.start(t)
			var req struct {
				Units []workset.ExecutionUnit `json:"units"`
			}
			if err := json.Unmarshal([]byte(ex.RequestJSON), &req); err != nil {
				t.Fatal(err)
			}
			first := req.Units[0]
			var o dirrename.Outcome
			if err := json.Unmarshal(first.Payload, &o); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(detailRoot(f.member), o.TargetName)
			// An unstarted later unit must not be inspected or diagnosed on this pass.
			later := first
			later.Index = 1
			later.RootIndex = 1
			later.ID = "later"
			later.RootPath = filepath.Join(detailRoot(f.member), "missing unstarted unit")
			req.Units = append(req.Units, later)
			raw, _ := json.Marshal(req)
			if _, writeErr := f.repo.DB().
				Exec(`UPDATE plan_executions SET total_components=2,total_operations=2 WHERE execution_id=?`, ex.ExecutionID); writeErr != nil {
				t.Fatal(writeErr)
			}
			if _, writeErr := f.repo.DB().
				Exec(`UPDATE plan_execution_requests SET request_json=? WHERE execution_id=?`, string(raw), ex.ExecutionID); writeErr != nil {
				t.Fatal(writeErr)
			}
			if err := filesystem.RenameNoReplace(f.member.FolderPath, target); err != nil {
				t.Fatal(err)
			}
			if synced {
				if err := f.repo.RelocateDirectory(
					t.Context(),
					detailRoot(f.member),
					f.member.FolderPath,
					target,
				); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.repo.InterruptStaleExecutions(); err != nil {
				t.Fatal(err)
			}
			if errs := f.task.RecoverInterrupted(t.Context(), f.repo); len(errs) > 0 {
				t.Fatal(errs)
			}
			assertRecoveredResult(t, f, ex.ExecutionID)
			// Keep the original request at one unit to verify repeated repair is a no-op.
			req.Units = req.Units[:1]
			raw, _ = json.Marshal(req)
			if _, writeErr := f.repo.DB().
				Exec(`UPDATE plan_execution_requests SET request_json=? WHERE execution_id=?`, string(raw), ex.ExecutionID); writeErr != nil {
				t.Fatal(writeErr)
			}
			if errs := f.task.RecoverInterrupted(t.Context(), f.repo); len(errs) > 0 {
				t.Fatal(errs)
			}
			current, err := f.repo.GetExecution(ex.ExecutionID)
			if err != nil || current.Status != workset.ExecStatusInterrupted || current.CompletedComponents != 1 ||
				current.CompletedOperations != 1 ||
				current.CurrentRoot != "" ||
				current.CurrentPhase != "" {
				t.Fatalf("execution: %+v %v", current, err)
			}
		})
	}
}
func TestRepairFailureCanRetryAndPersistedRollbackFailureIsSkipped(t *testing.T) {
	f := newRenameRun(t)
	ex := f.start(t)
	target := filepath.Join(detailRoot(f.member), "[RJ00000001] title")
	if err := filesystem.RenameNoReplace(f.member.FolderPath, target); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.InterruptStaleExecutions(); err != nil {
		t.Fatal(err)
	}
	inventoryfixture.SeedInventory(t, f.repo, inventoryfixture.Row{Path: target})
	if errs := f.task.RecoverInterrupted(t.Context(), f.repo); len(errs) != 1 {
		t.Fatalf("missing conflict: %v", errs)
	}
	rows, err := f.repo.ListExecutionComponentResults(ex.ExecutionID, 0, 0)
	if err != nil || len(rows) != 0 {
		t.Fatalf("failed repair saved result: %+v %v", rows, err)
	}
	if err := f.repo.SyncObservedInventory("", []string{target}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if errs := f.task.RecoverInterrupted(t.Context(), f.repo); len(errs) > 0 {
		t.Fatal(errs)
	}
	// A saved rollback failure remains a historical result and is never retried.
	if err := f.repo.SaveExecutionComponentResult(
		ex.ExecutionID,
		workset.ExecutionComponentResult{
			ComponentIndex: 0,
			Status:         workset.ExecComponentFailed,
			ResultJSON:     `{"error_code":"RENAME_ROLLBACK_FAILED"}`,
		},
		workset.ExecutionPosition{},
	); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if errs := f.task.RecoverInterrupted(t.Context(), f.repo); len(errs) > 0 {
		t.Fatal("revisited saved failure", errs)
	}
}
func TestUnstartedInterruptedRenameHasNoDiagnosticOrResult(t *testing.T) {
	f := newRenameRun(t)
	ex := f.start(t)
	if _, err := f.repo.InterruptStaleExecutions(); err != nil {
		t.Fatal(err)
	}
	if errs := f.task.RecoverInterrupted(t.Context(), f.repo); len(errs) > 0 {
		t.Fatal(errs)
	}
	rows, err := f.repo.ListExecutionComponentResults(ex.ExecutionID, 0, 0)
	if err != nil || len(rows) != 0 {
		t.Fatalf("unstarted: %+v %v", rows, err)
	}
}

func assertRecoveredResult(t *testing.T, f renameRun, id string) {
	t.Helper()
	rows, err := f.repo.ListExecutionComponentResults(id, 0, 0)
	if err != nil || len(rows) != 1 || rows[0].Status != workset.ExecComponentSucceeded {
		t.Fatalf("results: %+v %v", rows, err)
	}
	var facts struct {
		TaskResult      dirrename.Result `json:"task_result"`
		InventorySynced bool             `json:"inventory_synced"`
	}
	if err = json.Unmarshal([]byte(rows[0].ResultJSON), &facts); err != nil {
		t.Fatal(err)
	}
	if !facts.TaskResult.Renamed || !facts.TaskResult.Recovered || !facts.InventorySynced {
		t.Fatalf("facts: %+v", facts)
	}
}

type failingRecoveryResult struct {
	*sqlite.Repository

	fail bool
}

func (r *failingRecoveryResult) SaveExecutionComponentResult(
	id string,
	res workset.ExecutionComponentResult,
	pos workset.ExecutionPosition,
) error {
	if r.fail {
		return errors.New("result write failed")
	}
	return r.Repository.SaveExecutionComponentResult(id, res, pos)
}
func TestRepairRetriesResultPersistenceAfterSuccessfulRelocation(t *testing.T) {
	f := newRenameRun(t)
	ex := f.start(t)
	target := filepath.Join(detailRoot(f.member), "[RJ00000001] title")
	if err := filesystem.RenameNoReplace(f.member.FolderPath, target); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.InterruptStaleExecutions(); err != nil {
		t.Fatal(err)
	}
	store := &failingRecoveryResult{Repository: f.repo, fail: true}
	if errs := f.task.RecoverInterrupted(t.Context(), store); len(errs) != 1 {
		t.Fatalf("missing write failure: %v", errs)
	}
	rows, err := f.repo.ListExecutionComponentResults(ex.ExecutionID, 0, 0)
	if err != nil || len(rows) != 0 {
		t.Fatalf("saved failed result: %+v %v", rows, err)
	}
	store.fail = false
	if errs := f.task.RecoverInterrupted(t.Context(), store); len(errs) > 0 {
		t.Fatal(errs)
	}
	assertRecoveredResult(t, f, ex.ExecutionID)
}
