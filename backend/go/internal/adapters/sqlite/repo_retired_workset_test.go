package sqlite_test

import (
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestReplacingRecordRetiresAndHidesItsWholeState(t *testing.T) {
	for _, kind := range []string{"conversion", "rename", "metadata_refresh"} {
		t.Run(kind, func(t *testing.T) {
			repo, _ := retiredPlanFixture(t, kind)
			for _, statement := range []string{
				`INSERT INTO libraries(id,name,root_path) VALUES('lib','Library','/music')`,
				`UPDATE worksets SET library_id='lib',creation_idem_key='old-key' WHERE id='set'`,
				`INSERT INTO workset_members(workset_id,member_index,rel_path) VALUES('set',0,'RJ1')`,
				`INSERT INTO workset_operation_drafts(workset_id,operation_type) SELECT workset_id,operation_type FROM workset_operations`,
				`INSERT INTO cover_deliveries(id,execution_id,work_code,locale,copy_id,target_json,refresh,status) VALUES('cover','exec','RJ1','','copy','{}',0,'running')`,
			} {
				if _, err := repo.DB().Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			guarded, notified := false, false
			repo.PlanRetirementGuard = func(write func() error) error { guarded = true; return write() }
			repo.OnPlansRetired = func() { notified = true }
			replaceFixtureRecord(t, repo)
			var n int
			if err := repo.DB().
				QueryRow(`SELECT COUNT(*) FROM plans WHERE plan_id='old'`).
				Scan(&n); err != nil ||
				n != 1 {
				t.Fatalf("replacement synchronously deleted old payload: %d, %v", n, err)
			}
			if !guarded || !notified {
				t.Fatalf("retirement guard=%v cleanup wake=%v", guarded, notified)
			}
			assertRetiredRecordHidden(t, repo, kind)
			if err := repo.CreateGeneration(
				&workset.PlanGeneration{
					GenerationID:  "late",
					WorksetID:     "set",
					OperationType: kind,
					CreatedAt:     time.Now(),
				},
			); !errors.Is(
				err,
				workset.ErrWorksetNotFound,
			) {
				t.Fatalf("generation admitted after record retirement: %v", err)
			}

			assertRetiredPlanHidden(t, repo, kind)
			if _, err := repo.DB().Exec(`UPDATE worksets SET library_id=NULL WHERE id='set'`); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateExecutionGuarded(
				&workset.PlanExecution{
					ExecutionID:   "refused",
					WorksetID:     "set",
					OperationType: kind,
					PlanID:        "old",
					CreatedAt:     time.Now(),
				},
				workset.ExecutionGuards{},
			); !errors.Is(
				err,
				workset.ErrWorksetNotFound,
			) {
				t.Fatalf("retired record classified as available/orphaned: %v", err)
			}

			if ok, err := repo.CoverCurrent(t.Context(), []string{"cover"}); err != nil || ok {
				t.Fatalf("claimed cover remains publishable: %v, %v", ok, err)
			}
			if job, err := repo.ClaimCover(t.Context()); err != nil || job != nil {
				t.Fatalf("retired cover claimed: %+v, %v", job, err)
			}
		})
	}
}

func replaceFixtureRecord(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	now := time.Now()
	if err := repo.ReplaceCurrentWorkset(
		&workset.Workset{ID: "new-set", LibraryID: "lib", Version: 1, CreatedAt: now, UpdatedAt: now},
		nil,
		nil,
		nil,
		"set",
	); err != nil {
		t.Fatal(err)
	}
}

func TestRetiredRecordCleanupBoundsEachTableAndPreservesCurrentRecord(t *testing.T) {
	repo, path := retiredPlanFixture(t, "conversion")
	for _, q := range []string{
		`INSERT INTO libraries(id,name,root_path) VALUES('lib','Library','/music')`,
		`UPDATE worksets SET library_id='lib'`,
		`INSERT INTO workset_operation_drafts(workset_id,operation_type) VALUES('set','conversion')`,
		`INSERT INTO cover_deliveries(id,execution_id,work_code,locale,copy_id,target_json,refresh) VALUES('cover','exec','RJ1','','copy','{}',0)`,
	} {
		if _, err := repo.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"rename", "metadata_refresh"} {
		for _, query := range []string{
			`INSERT INTO workset_operations(workset_id,operation_type,current_revision_id) VALUES('set','%[1]s','%[1]s')`,
			`INSERT INTO workset_operation_drafts(workset_id,operation_type) VALUES('set','%[1]s')`,
			`INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id) VALUES('%[1]s','/music','snapshot','set')`,
			`INSERT INTO workset_operation_revisions(plan_id,workset_id,operation_type,revision_index) VALUES('%[1]s','set','%[1]s',1)`,
			`INSERT INTO plan_generations(generation_id,workset_id,operation_type,status,revision_id) VALUES('%[1]s','set','%[1]s','completed','%[1]s')`,
			`INSERT INTO plan_executions(execution_id,workset_id,operation_type,plan_id,status) VALUES('%[1]s','set','%[1]s','%[1]s','succeeded')`,
			`INSERT INTO execution_component_results(execution_id,component_index) VALUES('%[1]s',0)`,
			`INSERT INTO cover_deliveries(id,execution_id,work_code,locale,copy_id,target_json,refresh) VALUES('%[1]s','%[1]s','RJ1','','copy','{}',0)`,
		} {
			if _, err := repo.DB().Exec(fmt.Sprintf(query, kind)); err != nil {
				t.Fatal(err)
			}
		}
	}
	tx, err := repo.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := range 1003 {
		if _, err = tx.Exec(`INSERT INTO plan_member_conclusions(plan_id,member_id) VALUES('old',?)`, i); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(
			`INSERT INTO workset_members(workset_id,member_index,rel_path) VALUES('set',?,?)`,
			i,
			strconv.Itoa(i),
		); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(
			`INSERT INTO execution_component_results(execution_id,component_index) VALUES('exec',?)`,
			i,
		); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	replaceFixtureRecord(t, repo)
	counts := func() map[string]int {
		out := retirementTableCounts(t, repo)
		for _, table := range []string{"workset_members", "workset_operation_drafts", "plan_generations", "workset_operations", "worksets"} {
			var n int
			if err := repo.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
				t.Fatal(err)
			}
			out[table] = n
		}
		return out
	}
	drainRetiredRecord(t, &repo, path, counts)
}

func drainRetiredRecord(t *testing.T, repository **sqlite.Repository, path string, counts func() map[string]int) {
	t.Helper()
	repo := *repository
	before := counts()
	for batch := range 50 {
		n, err := repo.DeleteRetiredPlanBatch(t.Context(), 1000)
		if err != nil {
			t.Fatal(err)
		}
		after := counts()
		changed, removed := 0, 0
		for table, old := range before {
			if delta := old - after[table]; delta != 0 {
				changed++
				removed += delta
			}
		}
		if n == 0 {
			for table, count := range after {
				if table == "worksets" {
					count--
				}
				if count != 0 {
					t.Fatalf("cleanup left %s=%d", table, count)
				}
			}
			if _, readErr := repo.GetWorkset("new-set"); readErr != nil {
				t.Fatal(readErr)
			}
			return
		}
		if changed != 1 || n > 1000 || int64(removed) != n {
			t.Fatalf("unbounded batch rows=%d changed=%d removed=%d", n, changed, removed)
		}
		before = after
		if batch == 0 {
			if err = repo.Close(); err != nil {
				t.Fatal(err)
			}
			repo, err = sqlite.NewDevelopmentRepository(path)
			*repository = repo
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repo.Close() })
		}
	}
	t.Fatal("cleanup did not terminate")
}

func assertRetiredRecordHidden(t *testing.T, repo *sqlite.Repository, kind string) {
	t.Helper()
	if _, err := repo.GetWorkset("set"); !errors.Is(err, workset.ErrWorksetNotFound) {
		t.Fatalf("retired record visible: %v", err)
	}
	for _, includeOrphaned := range []bool{false, true} {
		rows, err := repo.ListWorksets("", "", 50, "", includeOrphaned)
		if err != nil || len(rows) != 1 || rows[0].ID != "new-set" {
			t.Fatalf("record list: %+v, %v", rows, err)
		}
	}
	current, err := repo.GetCurrentWorkset("lib")
	if err != nil || current == nil || current.ID != "new-set" {
		t.Fatalf("current: %+v, %v", current, err)
	}
	if n, err := repo.CountWorksetsForLibrary("lib"); err != nil || n != 1 {
		t.Fatalf("visible count=%d, %v", n, err)
	}
	if members, err := repo.ListWorksetMembers("set"); err != nil || len(members) != 0 {
		t.Fatalf("retired members: %+v, %v", members, err)
	}
	if ops, err := repo.ListOperations("set"); err != nil || len(ops) != 0 {
		t.Fatalf("retired operations: %+v, %v", ops, err)
	}
	if _, err := repo.GetOperation("set", kind); !errors.Is(err, workset.ErrOperationNotFound) {
		t.Fatalf("retired operation: %v", err)
	}
	if draft, err := repo.GetOperationDraft("set", kind); err != nil || draft != nil {
		t.Fatalf("retired draft: %+v, %v", draft, err)
	}
	if _, err := repo.GetGeneration("first"); !errors.Is(err, workset.ErrGenerationNotFound) {
		t.Fatalf("retired generation: %v", err)
	}
}
