package app_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/app"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestRetiredPlanWorkerStartupCommitAndMaintenanceTriggers(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if _, err = repo.DB().Exec(`INSERT INTO worksets(id) VALUES('set');
		INSERT INTO workset_operations(workset_id,operation_type,current_revision_id) VALUES('set','conversion','old');
		INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id,status) VALUES('startup','/music','s','set','retired');
		INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id) VALUES('old','/music','s','set')`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	kick, done := app.StartRetiredPlanCleanup(ctx, repo)
	t.Cleanup(func() { cancel(); <-done })
	assertPlanRows(t, repo, "startup", 0) // startup drains before returning
	if err = repo.CreateGeneration(&workset.PlanGeneration{
		GenerationID: "gen", WorksetID: "set", OperationType: "conversion", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err = repo.PersistOperationRevision("gen", "set", "conversion", time.Now(), workset.OperationRevisionPersist{
		PlanID: "new", RootPath: "/music", SnapshotToken: "s-new",
	}); err != nil {
		t.Fatal(err)
	}
	waitForPlanDeletion(t, repo, "old") // commit wakes the worker without a timer
	if _, err = repo.DB().
		Exec(`INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id,status) VALUES('maintenance','/music','s','set','retired')`); err != nil {
		t.Fatal(err)
	}
	loop := app.NewMaintenanceLoop(repo, func() (func(), error) {
		return nil, &admission.BusyError{Reason: "manual file operation"}
	}, app.Options{RetiredCleanup: kick})
	if loop.Pass(t.Context()) {
		t.Fatal("ordinary maintenance should be refused")
	}
	waitForPlanDeletion(t, repo, "maintenance") // invisible cleanup needs no slot
	assertPlanRows(t, repo, "new", 1)
}

func waitForPlanDeletion(t *testing.T, repo *sqlite.Repository, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var rows int
		if err := repo.DB().QueryRow(`SELECT COUNT(*) FROM plans WHERE plan_id=?`, id).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("worker did not delete %s", id)
		}
		time.Sleep(time.Millisecond) // wait for real SQLite I/O
	}
}

func assertPlanRows(t *testing.T, repo *sqlite.Repository, id string, want int) {
	t.Helper()
	var rows int
	if err := repo.DB().
		QueryRow(`SELECT COUNT(*) FROM plans WHERE plan_id=?`, id).
		Scan(&rows); err != nil ||
		rows != want {
		t.Fatalf("plan %s rows=%d want=%d err=%v", id, rows, want, err)
	}
}
