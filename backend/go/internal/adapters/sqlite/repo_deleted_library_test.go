package sqlite_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/library"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestDeletingLibraryRetiresAndHidesItsRecordUntilCleanup(t *testing.T) {
	for _, kind := range []string{"conversion", "rename", "metadata_refresh"} {
		t.Run(kind, func(t *testing.T) {
			repo, _ := retiredPlanFixture(t, kind)
			for _, q := range []string{
				`INSERT INTO libraries(id,name,root_path) VALUES('lib','Library','/music')`,
				`UPDATE worksets SET library_id='lib',creation_idem_key='old-key'`,
				`INSERT INTO workset_members(workset_id,member_index,rel_path) VALUES('set',0,'RJ1')`,
				`INSERT INTO workset_operation_drafts(workset_id,operation_type) SELECT workset_id,operation_type FROM workset_operations`,
				`INSERT INTO cover_deliveries(id,execution_id,work_code,locale,copy_id,target_json,refresh,status)
				 VALUES('cover','exec','RJ1','','copy','{}',0,'running')`,
			} {
				if _, err := repo.DB().Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			guarded, notified := false, false
			repo.PlanRetirementGuard = func(write func() error) error { guarded = true; return write() }
			repo.OnPlansRetired = func() {
				notified = true
				var retired bool
				if err := repo.DB().
					QueryRow(`SELECT retired=1 AND library_id IS NULL FROM worksets WHERE id='set'`).
					Scan(&retired); err != nil || !retired {
					t.Errorf("cleanup notified before deletion committed: %v, %v", retired, err)
				}
			}
			if err := repo.DeleteLibrary("lib"); err != nil {
				t.Fatal(err)
			}
			if !guarded || !notified {
				t.Fatalf("publication guard=%v cleanup wake=%v", guarded, notified)
			}
			assertDeletedLibraryRecordHidden(t, repo, kind)
			assertRetiredPlanHidden(t, repo, kind)
			if ok, err := repo.CoverCurrent(t.Context(), []string{"cover"}); err != nil || ok {
				t.Fatalf("claimed cover remained publishable: %v, %v", ok, err)
			}
			if job, err := repo.ClaimCover(t.Context()); err != nil || job != nil {
				t.Fatalf("retired cover claimed: %+v, %v", job, err)
			}
			assertDeletedRecordCleaned(t, repo)
		})
	}
}

func assertDeletedLibraryRecordHidden(t *testing.T, repo *sqlite.Repository, kind string) {
	t.Helper()
	if _, err := repo.GetWorkset("set"); !errors.Is(err, workset.ErrWorksetNotFound) {
		t.Fatalf("deleted record visible: %v", err)
	}
	for _, includeOrphaned := range []bool{false, true} {
		rows, err := repo.ListWorksets("", "", 50, "", includeOrphaned)
		if err != nil || len(rows) != 0 {
			t.Fatalf("deleted record listed (includeOrphaned=%v): %+v, %v", includeOrphaned, rows, err)
		}
	}
	if current, err := repo.GetCurrentWorkset("lib"); err != nil || current != nil {
		t.Fatalf("deleted current record: %+v, %v", current, err)
	}
	if n, err := repo.CountWorksetsForLibrary("lib"); err != nil || n != 0 {
		t.Fatalf("deleted library record count=%d, %v", n, err)
	}
	if members, err := repo.ListWorksetMembers("set"); err != nil || len(members) != 0 {
		t.Fatalf("deleted members: %+v, %v", members, err)
	}
	if _, err := repo.GetOperation("set", kind); !errors.Is(err, workset.ErrOperationNotFound) {
		t.Fatalf("deleted operation: %v", err)
	}
	if draft, err := repo.GetOperationDraft("set", kind); err != nil || draft != nil {
		t.Fatalf("deleted draft: %+v, %v", draft, err)
	}
	if _, err := repo.GetGeneration("first"); !errors.Is(err, workset.ErrGenerationNotFound) {
		t.Fatalf("deleted generation: %v", err)
	}
	if err := repo.CreateExecutionGuarded(&workset.PlanExecution{
		ExecutionID: "late", WorksetID: "set", OperationType: kind, PlanID: "old", CreatedAt: time.Now(),
	}, workset.ExecutionGuards{}); !errors.Is(err, workset.ErrWorksetNotFound) {
		t.Fatalf("deleted record classified as available/orphaned: %v", err)
	}
}

func TestDeletingLibraryPreservesActiveSessionGuards(t *testing.T) {
	for _, session := range []string{"generation", "execution"} {
		for _, status := range []string{"queued", "running"} {
			t.Run(session+"/"+status, func(t *testing.T) {
				repo, _ := retiredPlanFixture(t, "conversion")
				if _, err := repo.DB().Exec(`INSERT INTO libraries(id,name,root_path) VALUES('lib','Library','/music');
				 UPDATE worksets SET library_id='lib'`); err != nil {
					t.Fatal(err)
				}
				query, want := `UPDATE plan_generations SET status=?`, library.ErrGenerationInProgress
				if session == "execution" {
					query, want = `UPDATE plan_executions SET status=?`, library.ErrExecutionInProgress
				}
				if _, err := repo.DB().Exec(query, status); err != nil {
					t.Fatal(err)
				}
				repo.OnPlansRetired = func() { t.Error("refused delete woke cleanup") }
				if err := repo.DeleteLibrary("lib"); !errors.Is(err, want) {
					t.Fatalf("delete error=%v, want %v", err, want)
				}
				if _, err := repo.GetLibrary("lib"); err != nil {
					t.Fatalf("refused delete removed library: %v", err)
				}
				if _, err := repo.GetWorkset("set"); err != nil {
					t.Fatalf("refused delete retired record: %v", err)
				}
			})
		}
	}
}

func assertDeletedRecordCleaned(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	for batch := range 50 {
		n, err := repo.DeleteRetiredPlanBatch(t.Context(), 1)
		if err != nil || n > 1 {
			t.Fatalf("cleanup rows=%d, %v", n, err)
		}
		if n == 0 {
			break
		}
		if batch == 49 {
			t.Fatal("cleanup did not finish")
		}
	}
	counts := retirementTableCounts(t, repo)
	for _, table := range []string{"worksets", "workset_members", "workset_operation_drafts", "workset_operations", "plan_generations"} {
		var n int
		if err := repo.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		counts[table] = n
	}
	for table, n := range counts {
		if n != 0 {
			t.Errorf("cleanup left %s=%d", table, n)
		}
	}
}
