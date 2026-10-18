package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

func TestCoverQueueSharesDownloadsAndRecoversWithoutRevivingCanceledRequests(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ctx := t.Context()
	if _, err = repo.DB().
		Exec(`INSERT INTO worksets(id) VALUES('set'); INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('exec-a','set','plan-a'),('exec-b','set','plan-b');`); err != nil {
		t.Fatal(err)
	}
	req := coverdelivery.Request{
		ExecutionID: "exec-a",
		WorkCode:    "RJ00000001",
		URL:         "https://img.dlsite.jp/main.webp",
		Locale:      "ja-jp",
		Targets:     []coverdelivery.Target{{CopyID: "copy-a", WorkCode: "RJ00000001"}},
	}
	if err = repo.EnqueueCovers(ctx, req); err != nil {
		t.Fatal(err)
	}
	req.ExecutionID = "exec-b"
	req.Targets[0].CopyID = "copy-b"
	if err = repo.EnqueueCovers(ctx, req); err != nil {
		t.Fatal(err)
	}
	job, err := repo.ClaimCover(ctx)
	if err != nil || job == nil {
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
	if err != nil || recovered == nil || recovered.ID != job.ID {
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
