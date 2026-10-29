package sqlite_test

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/cleanup"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func failStagingMidway(t *testing.T, repo *sqlite.Repository, genID, planID string) {
	t.Helper()
	if _, err := repo.DB().
		Exec(`CREATE TRIGGER fail_staging BEFORE INSERT ON plan_roots WHEN NEW.root_index=150 BEGIN SELECT RAISE(ABORT,'injected staging failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := repo.PersistOperationRevision(
		t.Context(),
		genID,
		"set",
		"conversion",
		time.Now(),
		stagedPayload(planID),
	); err == nil {
		t.Fatal("injected failure succeeded")
	}
	if _, err := repo.DB().Exec(`DROP TRIGGER fail_staging`); err != nil {
		t.Fatal(err)
	}
}

func drainStagedPlans(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	for {
		before := retirementTableCounts(t, repo)
		n, err := repo.DeleteRetiredPlanBatch(t.Context(), 50)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			return
		}
		after := retirementTableCounts(t, repo)
		changed := 0
		for table, count := range before {
			if delta := count - after[table]; delta != 0 {
				changed++
				if delta < 0 || delta > 50 {
					t.Fatalf("unbounded delete in %s: %d", table, delta)
				}
			}
		}
		if changed != 1 {
			t.Fatalf("batch changed %d tables", changed)
		}
	}
}

func TestFailedStagingCanBeCleanedDuringImmediateRetry(t *testing.T) {
	repo, _ := retiredPlanFixture(t, "conversion")
	stagedGeneration(t, repo, "second", "conversion")
	failStagingMidway(t, repo, "second", "abandoned")
	if n, err := repo.DeleteRetiredPlanBatch(t.Context(), 50); err != nil || n != 0 {
		t.Fatalf("running staging deleted: %d %v", n, err)
	}
	if err := repo.MarkGenerationFailed("second", "PERSIST_FAILED", "injected"); err != nil {
		t.Fatal(err)
	}
	// Session retention must not erase the terminal owner before staged cleanup.
	candidates, err := repo.CleanupCandidates(t.Context(), cleanup.PlanGenerations, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range candidates {
		if id == "second" {
			t.Fatal("cleanup preview exposes staged owner")
		}
	}
	stagedGeneration(t, repo, "retry", "conversion")
	repo.PlanRetirementGuard = func(write func() error) error {
		drainStagedPlans(t, repo)
		var n int
		if err := repo.DB().
			QueryRow(`SELECT COUNT(*) FROM plans WHERE plan_id='abandoned'`).
			Scan(&n); err != nil ||
			n != 0 {
			t.Fatalf("abandoned staging retained: %d %v", n, err)
		}
		return write()
	}
	if err := repo.PersistOperationRevision(
		t.Context(),
		"retry",
		"set",
		"conversion",
		time.Now(),
		stagedPayload("retry-plan"),
	); err != nil {
		t.Fatal(err)
	}
	if detail, err := repo.GetPlanDetail("retry-plan"); err != nil || len(detail.Roots) != 251 {
		t.Fatalf("retry damaged: %+v %v", detail, err)
	}
}

func TestStagingProcessCrashRecovery(t *testing.T) {
	if path := os.Getenv("PREEN_STAGING_CRASH_DB"); path != "" {
		repo, err := sqlite.NewDevelopmentRepository(path)
		if err != nil {
			t.Fatal(err)
		}
		stagedGeneration(t, repo, "second", "conversion")
		failStagingMidway(t, repo, "second", "abandoned")
		fmt.Fprintln(os.Stdout, "staging persisted")
		for {
			time.Sleep(time.Hour)
		} // Parent kills this process with committed staging.
	}
	repo, path := retiredPlanFixture(t, "conversion")
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.CommandContext(t.Context(), executable, "-test.run=^TestStagingProcessCrashRecovery$")
	child.Env = append(os.Environ(), "PREEN_STAGING_CRASH_DB="+path)
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	scanner := bufio.NewScanner(output)
	if !scanner.Scan() || scanner.Text() != "staging persisted" {
		t.Fatalf("child did not persist staging: %s %v", scanner.Text(), scanner.Err())
	}
	if err = child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	repo, err = sqlite.NewDevelopmentRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if _, err = repo.GetPlanDetail("old"); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.GetPlanDetail("abandoned"); err == nil {
		t.Fatal("half plan visible after crash")
	}
	if n, deleteErr := repo.DeleteRetiredPlanBatch(t.Context(), 50); deleteErr != nil || n != 0 {
		t.Fatalf("stale generation not interrupted yet: %d %v", n, deleteErr)
	}
	if err = repo.InterruptStaleGenerations(); err != nil {
		t.Fatal(err)
	}
	drainStagedPlans(t, repo)
	var n int
	if err = repo.DB().QueryRow(`SELECT COUNT(*) FROM plans WHERE plan_id='abandoned'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("crash residue: %d %v", n, err)
	}
	if gen, readErr := repo.GetGeneration("second"); readErr != nil || gen.Status != workset.GenStatusInterrupted {
		t.Fatalf("recovered session: %+v %v", gen, readErr)
	}
}

func TestUnsuccessfulStagingPreservesOldExecutionEligibility(t *testing.T) {
	for _, outcome := range []string{"failed", "canceled", "interrupted"} {
		t.Run(outcome, func(t *testing.T) {
			repo, _ := retiredPlanFixture(t, "conversion")
			if _, err := repo.DB().Exec(`DELETE FROM plan_executions WHERE execution_id='exec';
    INSERT INTO libraries(id,root_path,root_path_key) VALUES('library','/music','/music');
    UPDATE worksets SET library_id='library';
    INSERT INTO workset_operation_drafts(workset_id,operation_type,draft_hash) VALUES('set','conversion','draft')`); err != nil {
				t.Fatal(err)
			}
			stagedGeneration(t, repo, "second", "conversion")
			failStagingMidway(t, repo, "second", "abandoned")
			execution := &workset.PlanExecution{
				ExecutionID:   "old-execution",
				WorksetID:     "set",
				OperationType: "conversion",
				PlanID:        "old",
				CreatedAt:     time.Now(),
			}
			guards := workset.ExecutionGuards{
				ExpectedOperationVersion: 1,
				ExpectedCurrentRevision:  "old",
				ExpectedDraftHash:        "draft",
			}
			if err := repo.CreateExecutionGuarded(execution, guards); err == nil {
				t.Fatal("running generation allowed execution")
			}
			var err error
			switch outcome {
			case "failed":
				err = repo.MarkGenerationFailed("second", "PERSIST_FAILED", "injected")
			case "canceled":
				err = repo.CompleteGenerationCanceled("second")
			case "interrupted":
				err = repo.MarkGenerationInterrupted("second")
			}
			if err != nil {
				t.Fatal(err)
			}
			drainStagedPlans(t, repo)
			if err = repo.CreateExecutionGuarded(execution, guards); err != nil {
				t.Fatalf("old revision lost execution eligibility: %v", err)
			}
			if err = repo.CreateExecutionGuarded(execution, guards); err == nil {
				t.Fatal("old revision executed twice")
			}
		})
	}
}

func TestStagingCleanupRequiresTerminalOwnerEvenAfterCancelRequest(t *testing.T) {
	for _, status := range []string{"queued", "running", "failed", "canceled", "interrupted", "completed"} {
		t.Run(status, func(t *testing.T) {
			repo, _ := retiredPlanFixture(t, "conversion")
			if _, err := repo.DB().
				Exec(`INSERT INTO plan_generations(generation_id,workset_id,operation_type,status,cancel_requested) VALUES('second','set','conversion',?,1)`, status); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.DB().
				Exec(`INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id,status,generation_id) VALUES('abandoned','/music','s','set','staging','second')`); err != nil {
				t.Fatal(err)
			}
			n, err := repo.DeleteRetiredPlanBatch(t.Context(), 50)
			want := int64(1)
			if status == "queued" || status == "running" {
				want = 0
			}
			if err != nil || n != want {
				t.Fatalf("cleanup status=%s deleted=%d want=%d: %v", status, n, want, err)
			}
		})
	}
}
