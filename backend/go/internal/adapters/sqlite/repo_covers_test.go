package sqlite_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

func newCoverRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if _, err = repo.DB().Exec(`
		INSERT INTO worksets(id) VALUES('set');
		INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id) VALUES('plan-a','/music','s','set'),('plan-b','/music','s','set');
		INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('exec-a','set','plan-a'),('exec-b','set','plan-b');
	`); err != nil {
		t.Fatal(err)
	}
	return repo
}

func enqueueCover(t *testing.T, repo *sqlite.Repository, execution, work, copyID string) {
	t.Helper()
	if err := repo.EnqueueCovers(t.Context(), coverdelivery.Request{
		ExecutionID: execution, WorkCode: work, Locale: "ja-jp",
		Targets: []coverdelivery.Target{{CopyID: copyID, WorkCode: work}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCoverQueueKeepsRowIDOrderAndClaimsWholeWork(t *testing.T) {
	repo := newCoverRepo(t)
	enqueueCover(t, repo, "exec-a", "RJ2", "c1")
	enqueueCover(t, repo, "exec-a", "RJ1", "c2")
	enqueueCover(t, repo, "exec-b", "RJ2", "c3")
	job, err := repo.ClaimCover(t.Context())
	if err != nil || job == nil || job.WorkCode != "RJ2" || len(job.Destinations) != 2 || job.Locale != "ja-jp" {
		t.Fatalf("claim=%+v err=%v", job, err)
	}
	job, err = repo.ClaimCover(t.Context())
	if err != nil || job == nil || job.WorkCode != "RJ1" || len(job.Destinations) != 1 {
		t.Fatalf("claim=%+v err=%v", job, err)
	}
	if job, err = repo.ClaimCover(t.Context()); err != nil || job != nil {
		t.Fatalf("empty queue claimed %+v %v", job, err)
	}
	rows, err := repo.DB().
		Query(`EXPLAIN QUERY PLAN SELECT 1 FROM cover_deliveries WHERE status='pending' AND work_code='RJ1'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(plan, " "); !strings.Contains(joined, "cover_deliveries_pending") {
		t.Fatalf("pending lookup plan: %v", plan)
	}
}

func TestCoverQueueSharesClaimsAndRecoversWithoutRevivingCanceledRequests(t *testing.T) {
	repo := newCoverRepo(t)
	ctx := t.Context()
	enqueueCover(t, repo, "exec-a", "RJ00000001", "copy-a")
	enqueueCover(t, repo, "exec-b", "RJ00000001", "copy-b")
	job, err := repo.ClaimCover(ctx)
	if err != nil || job == nil || len(job.Destinations) != 2 {
		t.Fatalf("claim: %v %v", job, err)
	}
	if other, e := repo.ClaimCover(ctx); e != nil || other != nil {
		t.Fatalf("duplicate download: %v %v", other, e)
	}
	if err = repo.ChangeCoverRequests(ctx, "exec-a", false); err != nil {
		t.Fatal(err)
	}
	if err = repo.RecoverCovers(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := repo.ClaimCover(ctx)
	if err != nil || recovered == nil || len(recovered.Destinations) != 1 ||
		recovered.Destinations[0].ExecutionID != "exec-b" {
		t.Fatalf("recovery lost work: %+v %v", recovered, err)
	}
	report, err := repo.CoverReport(ctx, "exec-a")
	if err != nil || report.Counts["canceled"] != 1 {
		t.Fatalf("cancellation revived: %+v %v", report, err)
	}
	report, err = repo.CoverReport(ctx, "exec-b")
	if err != nil || report.Counts["running"] != 1 {
		t.Fatalf("shared request canceled: %+v %v", report, err)
	}
}
