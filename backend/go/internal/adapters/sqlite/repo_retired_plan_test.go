package sqlite_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestReplacementRetiresEveryOperationAndRevokesCoverDemand(t *testing.T) {
	for _, kind := range []string{"conversion", "metadata_refresh", "rename"} {
		t.Run(kind, func(t *testing.T) {
			repo, _ := retiredPlanFixture(t, kind)
			if err := repo.EnqueueCovers(t.Context(), coverdelivery.Request{
				ExecutionID: "exec",
				WorkCode:    "RJ1",
				Targets:     []coverdelivery.Target{{CopyID: "pending"}, {CopyID: "running"}, {CopyID: "succeeded"}},
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.DB().Exec(`UPDATE cover_deliveries SET status=copy_id`); err != nil {
				t.Fatal(err)
			}
			retireFixturePlan(t, repo, kind)
			var status string
			if err := repo.DB().
				QueryRow(`SELECT status FROM plans WHERE plan_id='old'`).
				Scan(&status); err != nil ||
				status != "retired" {
				t.Fatalf("retirement marker: %q, %v", status, err)
			}
			assertRetiredPlanHidden(t, repo, kind)
			gen, err := repo.GetGeneration("first")
			if err != nil || gen.RevisionID != "" || gen.Status != workset.GenStatusCompleted {
				t.Fatalf("completed generation replay: %+v, %v", gen, err)
			}
			assertRetiredCoverDemandCanceled(t, repo)
			current, err := repo.GetOperation("set", kind)
			if err != nil || current.CurrentRevisionID != "new" {
				t.Fatalf("current revision: %+v, %v", current, err)
			}
			if _, err = repo.GetPlanDetail("new"); err != nil {
				t.Fatal(err)
			}
			// Replacing a plan used to delete its execution key synchronously.
			// Retained invisible rows must not delay reuse until background cleanup.
			if _, err = repo.DB().
				Exec(`INSERT INTO plan_executions(execution_id,workset_id,operation_type,plan_id,status,idempotency_key)
				VALUES('new-exec','set',?,'new','succeeded','exec-key')`, kind); err != nil {
				t.Fatalf("retired execution still reserved its key: %v", err)
			}
		})
	}
}

func assertRetiredCoverDemandCanceled(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	if err := repo.ChangeCoverRequests(t.Context(), "exec", true); err != nil {
		t.Fatal(err)
	}
	if err := repo.EnqueueCovers(t.Context(), coverdelivery.Request{
		ExecutionID: "exec", WorkCode: "RJ2", Targets: []coverdelivery.Target{{CopyID: "late"}},
	}); err != nil {
		t.Fatal(err)
	}
	job, err := repo.ClaimCover(t.Context())
	if err != nil || job != nil {
		t.Fatalf("retired cover demand claimed: %+v, %v", job, err)
	}
	var canceled, succeeded int
	if err = repo.DB().
		QueryRow(`SELECT SUM(status='canceled'),SUM(status='succeeded') FROM cover_deliveries`).
		Scan(&canceled, &succeeded); err != nil || canceled != 2 ||
		succeeded != 1 {
		t.Fatalf("cover outcomes: canceled=%d succeeded=%d err=%v", canceled, succeeded, err)
	}
}

func TestRetiredRenameIsExcludedFromStartupRepair(t *testing.T) {
	repo, _ := retiredPlanFixture(t, "rename")
	if _, err := repo.DB().
		Exec(`UPDATE plan_executions SET status='interrupted' WHERE execution_id='exec'`); err != nil {
		t.Fatal(err)
	}
	retireFixturePlan(t, repo, "rename")
	if executions, err := repo.InterruptedRenames(t.Context()); err != nil || len(executions) != 0 {
		t.Fatalf("retired rename reached startup repair: %+v, %v", executions, err)
	}
}

func TestRetiredExecutionCannotHoldAdmissionOrBeScheduled(t *testing.T) {
	repo, _ := retiredPlanFixture(t, "conversion")
	if _, err := repo.DB().Exec(`UPDATE worksets SET root_path='/music',root_path_key='/music';
		UPDATE plan_executions SET status='queued' WHERE execution_id='exec'`); err != nil {
		t.Fatal(err)
	}
	retireFixturePlan(t, repo, "conversion")
	active, err := repo.HasActiveSession()
	if err != nil || active {
		t.Fatalf("retired execution holds file admission: %v, %v", active, err)
	}
	active, err = repo.HasActiveExecutionForRoot("/music")
	if err != nil || active {
		t.Fatalf("retired execution holds root admission: %v, %v", active, err)
	}
	active, err = repo.HasActiveExecutionForRootKinds("/music", []string{"conversion"})
	if err != nil || active {
		t.Fatalf("retired execution holds task admission: %v, %v", active, err)
	}
	execution, err := repo.NextQueuedExecution()
	if err != nil || execution != nil {
		t.Fatalf("retired execution scheduled: %+v, %v", execution, err)
	}
}

func assertRetiredPlanHidden(t *testing.T, repo *sqlite.Repository, kind string) {
	t.Helper()
	if _, err := repo.GetPlanDetail("old"); !errors.Is(err, workset.ErrPlanNotFound) {
		t.Fatalf("retired payload visible: %v", err)
	}
	if _, err := repo.GetOperationRevision("set", kind, "old"); !errors.Is(err, workset.ErrRevisionNotFound) {
		t.Fatalf("retired revision visible: %v", err)
	}
	if roots, err := repo.GetPlanRoots("old"); err != nil || len(roots) != 0 {
		t.Fatalf("retired roots visible: %+v, %v", roots, err)
	}
	assertRetiredExecutionHidden(t, repo, kind)
}

func assertRetiredExecutionHidden(t *testing.T, repo *sqlite.Repository, kind string) {
	t.Helper()
	if _, err := repo.GetExecution("exec"); !errors.Is(err, workset.ErrExecutionNotFound) {
		t.Fatalf("retired execution visible: %v", err)
	}
	if _, err := repo.GetExecutionProgress("exec"); !errors.Is(err, workset.ErrExecutionNotFound) {
		t.Fatalf("retired execution progress visible: %v", err)
	}
	byKey, err := repo.GetExecutionByOperationKey("set", kind, "exec-key")
	if err != nil || byKey != nil {
		t.Fatalf("retired execution replays: %+v, %v", byKey, err)
	}
	latest, err := repo.LatestExecutionForOperation("set", kind)
	if err != nil || latest != nil {
		t.Fatalf("retired latest execution: %+v, %v", latest, err)
	}
	forRevision, err := repo.GetExecutionForRevision("old")
	if err != nil || forRevision != nil {
		t.Fatalf("retired revision execution: %+v, %v", forRevision, err)
	}
	results, err := repo.ListExecutionComponentResults("exec", 0, 0)
	if err != nil || len(results) != 0 {
		t.Fatalf("retired results visible: %+v, %v", results, err)
	}
	report, err := repo.CoverReport(t.Context(), "exec")
	if err != nil || len(report.Items) != 0 {
		t.Fatalf("retired covers visible: %+v, %v", report, err)
	}
}

func TestRetiredPlanCleanupBoundsEachTableAndResumesAfterReopen(t *testing.T) {
	repo, path := retiredPlanFixture(t, "conversion")
	tx, err := repo.DB().BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := range 1003 {
		if _, err = tx.ExecContext(
			t.Context(),
			`INSERT INTO cover_deliveries(id,execution_id,work_code,locale,copy_id,target_json,refresh) VALUES(?,'exec','RJ1','',?,'{}',0)`,
			strconv.Itoa(i),
			strconv.Itoa(i),
		); err != nil {
			t.Fatal(err)
		}
		for _, statement := range []string{
			`INSERT INTO execution_component_results(execution_id,component_index) VALUES('exec',?)`,
			`INSERT INTO conversion_components(plan_id,component_index,step_index,component_id) VALUES('old',?,0,'component')`,
			`INSERT INTO plan_roots(plan_id,root_index,root_path) VALUES('old',?,'/music')`,
			`INSERT INTO conversion_steps(plan_id,step_index,step_type) VALUES('old',?,'test')`,
		} {
			if _, err = tx.ExecContext(t.Context(), statement, i); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	retireFixturePlan(t, repo, "conversion")
	before := retirementTableCounts(t, repo)
	for batch := 0; ; batch++ {
		rows, deleteErr := repo.DeleteRetiredPlanBatch(t.Context(), 1000)
		if deleteErr != nil {
			t.Fatal(deleteErr)
		}
		after := retirementTableCounts(t, repo)
		changed, removed := 0, 0
		for table, old := range before {
			if delta := old - after[table]; delta != 0 {
				changed++
				removed += delta
			}
		}
		if rows == 0 {
			if changed != 0 || after["plans"] != 1 || after["workset_operation_revisions"] != 1 {
				t.Fatalf("cleanup incomplete: %+v", after)
			}
			break
		}
		if changed != 1 || rows > 1000 || int64(removed) != rows {
			t.Fatalf("unbounded/cascading batch: rows=%d tables=%d removed=%d", rows, changed, removed)
		}
		before = after
		if batch == 0 {
			if err = repo.Close(); err != nil {
				t.Fatal(err)
			}
			repo, err = sqlite.NewDevelopmentRepository(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repo.Close() })
		}
		if batch > 30 {
			t.Fatal("cleanup did not terminate")
		}
	}
	if _, err = repo.GetPlanDetail("new"); err != nil {
		t.Fatalf("cleanup removed current plan: %v", err)
	}
	var violations int
	if err = repo.DB().
		QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check`).
		Scan(&violations); err != nil ||
		violations != 0 {
		t.Fatalf("foreign key violations: %d, %v", violations, err)
	}
}

func retirementTableCounts(t *testing.T, repo *sqlite.Repository) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, table := range []string{"cover_deliveries", "execution_component_results", "plan_executions", "conversion_components", "plan_roots", "conversion_steps", "workset_operation_revisions", "plans"} {
		var count int
		if err := repo.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		counts[table] = count
	}
	return counts
}

func retiredPlanFixture(t *testing.T, kind string) (*sqlite.Repository, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cache.db")
	repo, err := sqlite.NewDevelopmentRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	for _, statement := range []string{
		`INSERT INTO worksets(id) VALUES('set')`,
		fmt.Sprintf(`INSERT INTO workset_operations(workset_id,operation_type,current_revision_id) VALUES('set','%s','old')`, kind),
		`INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id) VALUES('old','/music','snapshot','set')`,
		fmt.Sprintf(`INSERT INTO workset_operation_revisions(plan_id,workset_id,operation_type,revision_index) VALUES('old','set','%s',1)`, kind),
		fmt.Sprintf(`INSERT INTO plan_generations(generation_id,workset_id,operation_type,status,revision_id,idempotency_key) VALUES('first','set','%s','completed','old','first-key')`, kind),
		fmt.Sprintf(`INSERT INTO plan_executions(execution_id,workset_id,operation_type,plan_id,status,idempotency_key) VALUES('exec','set','%s','old','succeeded','exec-key')`, kind),
	} {
		if _, err = repo.DB().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return repo, path
}

func retireFixturePlan(t *testing.T, repo *sqlite.Repository, kind string) {
	t.Helper()
	if err := repo.CreateGeneration(&workset.PlanGeneration{
		GenerationID: "second", WorksetID: "set", OperationType: kind, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.NextQueuedGeneration(nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.PersistOperationRevision(
		t.Context(),
		"second",
		"set",
		kind,
		time.Now(),
		workset.OperationRevisionPersist{
			PlanID: "new", RootPath: "/music", SnapshotToken: "snapshot-new",
		},
	); err != nil {
		t.Fatal(err)
	}
}
