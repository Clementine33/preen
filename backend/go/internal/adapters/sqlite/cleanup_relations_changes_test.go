package sqlite_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/cleanup"
	"github.com/Clementine33/preen/backend/internal/works"
)

//nolint:gocognit // Each mutation is checked against the same stale-batch and counter contract.
func TestCleanupReportsRechecksCurrentRootsAndEvidence(t *testing.T) {
	for _, change := range []string{"new work", "changed root report", "new report dependency", "new independent report", "new cross-batch SCC"} {
		t.Run(change, func(t *testing.T) {
			repo := deletionRepository(t)
			saveCleanupReport(t, repo, "RJ00000002", works.ImportRelations{Role: "original"})
			saveCleanupReport(t, repo, "RJ00000003", works.ImportRelations{Role: "original"})
			ids, err := repo.CleanupCandidates(t.Context(), cleanup.RelationReports, time.Now(), nil)
			if err != nil || len(ids) != 2 {
				t.Fatalf("discovery: %v %v", ids, err)
			}
			switch change {
			case "new work":
				deletionSQL(
					t,
					repo,
					`INSERT INTO works(id,collected_at,updated_at) VALUES('RJ00000002','','')`,
				)
			case "changed root report":
				saveCleanupReport(
					t,
					repo,
					"RJ00000001",
					works.ImportRelations{Role: "child", ParentRJ: new("RJ00000002")},
				)
			case "new report dependency":
				saveCleanupReport(
					t,
					repo,
					"RJ00000004",
					works.ImportRelations{Role: "child", ParentRJ: new("RJ00000002")},
				)
			case "new independent report":
				saveCleanupReport(t, repo, "RJ00000004", works.ImportRelations{Role: "original"})
			case "new cross-batch SCC":
				report := works.ImportRelations{
					Role:      "child",
					ParentRJ:  new("RJ00000003"),
					ChildRJs:  []string{"RJ00000003"},
					Source:    "dlsite",
					FetchedAt: "2026-09-28T10:00:00.000000000Z",
				}
				if _, err := works.NewService(repo, nil, nil, nil, "").
					ImportRelationReport("RJ00000002", report); err != nil {
					t.Fatal(err)
				}
			}
			if createErr := repo.CreateCleanupRun(
				t.Context(),
				cleanup.Run{ID: "reports", Status: "running", Counts: map[cleanup.Category]cleanup.Count{}},
			); createErr != nil {
				t.Fatal(createErr)
			}
			// Exercise stale one-record batches captured before the concurrent change.
			for _, id := range ids {
				if batchErr := repo.CleanupBatch(
					t.Context(),
					"reports",
					cleanup.RelationReports,
					time.Now(),
					[]string{id},
				); batchErr != nil {
					t.Fatal(batchErr)
				}
				if violation := works.ValidateRelations(cleanupEvidence(t, repo)); violation != nil {
					t.Fatal(violation)
				}
			}
			run, err := repo.ReadCleanupRun(t.Context())
			want := cleanup.Count{Checked: 2, Deleted: 1, Skipped: 1}
			if change == "new independent report" {
				want = cleanup.Count{Checked: 2, Deleted: 2}
			}
			if change == "new cross-batch SCC" {
				want = cleanup.Count{Checked: 2, Skipped: 2}
			}
			if err != nil || run.Counts[cleanup.RelationReports] != want {
				t.Fatalf("run=%+v err=%v want=%+v", run, err, want)
			}
			ev := cleanupEvidence(t, repo)
			if change == "new independent report" || change == "new report dependency" {
				if _, exists := ev.Self["RJ00000004"]; !exists {
					t.Fatal("deleted new report outside discovery snapshot")
				}
			}
		})
	}
}

func TestCleanupReportFailureAndCancellationRollBack(t *testing.T) {
	for _, failure := range []string{"delete", "summary", "structure", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			repo := deletionRepository(t)
			saveCleanupReport(
				t,
				repo,
				"RJ00000002",
				works.ImportRelations{Role: "original", ChildRJs: []string{"RJ00000003"}},
			)
			if createErr := repo.CreateCleanupRun(
				t.Context(),
				cleanup.Run{ID: "reports", Status: "running", Counts: map[cleanup.Category]cleanup.Count{}},
			); createErr != nil {
				t.Fatal(createErr)
			}
			before := cleanupEvidence(t, repo)
			switch failure {
			case "delete":
				deletionSQL(
					t,
					repo,
					`CREATE TRIGGER fail_report_delete BEFORE DELETE ON relation_reports BEGIN SELECT RAISE(ABORT,'injected'); END`,
				)
			case "summary":
				deletionSQL(
					t,
					repo,
					`CREATE TRIGGER fail_report_count BEFORE UPDATE ON cleanup_run BEGIN SELECT RAISE(ABORT,'injected'); END`,
				)
			case "structure":
				deletionSQL(
					t,
					repo,
					`CREATE TRIGGER invalid_remaining_evidence AFTER DELETE ON relation_reports BEGIN INSERT INTO relation_reports VALUES('RJ00000003','child',NULL,NULL,'RJ00000003','dlsite',''); END`,
				)
			}
			notifications := 0
			repo.OnCatalogChanged = func() { notifications++ }
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if failure == "canceled" {
				cancel()
			}
			err := repo.CleanupBatch(ctx, "reports", cleanup.RelationReports, time.Now(), []string{"RJ00000002"})
			if err == nil {
				t.Fatal("failed batch succeeded")
			}
			if failure == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, cleanupEvidence(t, repo)) {
				t.Fatal("failed batch changed evidence")
			}
			run, err := repo.ReadCleanupRun(t.Context())
			if err != nil || run.Counts[cleanup.RelationReports] != (cleanup.Count{}) || notifications != 0 {
				t.Fatalf("run=%+v notifications=%d err=%v", run, notifications, err)
			}
		})
	}
}

func TestCleanupReportsWaitsForConcurrentProtectionCommit(t *testing.T) {
	for _, kind := range []string{"work", "report dependency"} {
		t.Run(kind, func(t *testing.T) {
			repo := deletionRepository(t)
			saveCleanupReport(t, repo, "RJ00000002", works.ImportRelations{Role: "original"})
			ids, err := repo.CleanupCandidates(t.Context(), cleanup.RelationReports, time.Now(), nil)
			if err != nil || len(ids) != 1 {
				t.Fatalf("discovery=%v %v", ids, err)
			}
			if createErr := repo.CreateCleanupRun(
				t.Context(),
				cleanup.Run{ID: "reports", Status: "running", Counts: map[cleanup.Category]cleanup.Count{}},
			); createErr != nil {
				t.Fatal(createErr)
			}
			tx, err := repo.DB().BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			query := `INSERT INTO works(id,collected_at,updated_at) VALUES('RJ00000002','','')`
			if kind == "report dependency" {
				query = `INSERT INTO relation_reports VALUES('RJ00000003','child',NULL,NULL,'RJ00000002','dlsite','')`
			}
			if _, execErr := tx.ExecContext(t.Context(), query); execErr != nil {
				t.Fatal(execErr)
			}
			started := make(chan struct{})
			deleted := make(chan error, 1)
			go func() {
				close(started)
				deleted <- repo.CleanupBatch(t.Context(), "reports", cleanup.RelationReports, time.Now(), ids)
			}()
			<-started
			if commitErr := tx.Commit(); commitErr != nil {
				t.Fatal(commitErr)
			}
			if batchErr := <-deleted; batchErr != nil {
				t.Fatal(batchErr)
			}
			run, err := repo.ReadCleanupRun(t.Context())
			if err != nil || run.Counts[cleanup.RelationReports] != (cleanup.Count{Checked: 1, Skipped: 1}) {
				t.Fatalf("run=%+v err=%v", run, err)
			}
			if _, exists := cleanupEvidence(t, repo).Self["RJ00000002"]; !exists {
				t.Fatal("deleted protected report after concurrent writer committed")
			}
		})
	}
}

func TestCleanupReportCancellationInsideDeleteRollsBackGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	function := reportMetricFunction(t, cancel)
	repo := deletionRepository(t)
	deletionSQL(
		t,
		repo,
		`INSERT INTO relation_reports VALUES('RJ00000002','child',NULL,NULL,'RJ00000003','dlsite',''),('RJ00000003','original',NULL,NULL,NULL,'dlsite',''); INSERT INTO relation_declared_children VALUES('RJ00000002','RJ00000003')`,
	)
	if err := repo.CreateCleanupRun(
		t.Context(),
		cleanup.Run{ID: "reports", Status: "running", Counts: map[cleanup.Category]cleanup.Count{}},
	); err != nil {
		t.Fatal(err)
	}
	deletionSQL(
		t,
		repo,
		`CREATE TRIGGER cancel_report_group BEFORE DELETE ON relation_reports BEGIN SELECT `+function+`(OLD.subject_rj); END`,
	)
	before := cleanupEvidence(t, repo)
	if err := repo.CleanupBatch(
		ctx,
		"reports",
		cleanup.RelationReports,
		time.Now(),
		[]string{"RJ00000002", "RJ00000003"},
	); err == nil {
		t.Fatal("canceled deletion committed")
	}
	if !reflect.DeepEqual(before, cleanupEvidence(t, repo)) {
		t.Fatal("cancellation split the group")
	}
	run, err := repo.ReadCleanupRun(t.Context())
	if err != nil || run.Counts[cleanup.RelationReports] != (cleanup.Count{}) {
		t.Fatalf("run=%+v err=%v", run, err)
	}
}
