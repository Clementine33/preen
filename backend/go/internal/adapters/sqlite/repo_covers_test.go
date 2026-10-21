package sqlite_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

func TestCoverRetentionKeepsEveryReferencedAndActiveDownload(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	_, err = repo.DB().Exec(`
		INSERT INTO worksets(id) VALUES('set');
		INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id) VALUES('plan','/music','s','set');
		INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('exec','set','plan');
		INSERT INTO cover_candidates VALUES('candidate','url','',1);
	`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ id, status, delivery string }{
		{"success", "done", "succeeded"}, {"failure", "done", "failed"}, {"canceled", "done", "canceled"},
		{"orphan-a", "done", ""}, {"orphan-b", "done", ""}, {"queued", "queued", ""}, {"running", "running", ""},
	} {
		_, err = repo.DB().Exec(`
			INSERT INTO cover_downloads(id,work_code,version,url,backup_url,locale,refresh,status)
			VALUES(?,?,1,'url','','ja',0,?)
		`, tc.id, tc.id, tc.status)
		if err != nil {
			t.Fatal(err)
		}
		if tc.delivery != "" {
			_, err = repo.DB().Exec(`
				INSERT INTO cover_deliveries(id,job_id,execution_id,copy_id,target_json,refresh,status)
				VALUES(?,?,'exec',?,'{}',0,?)
			`, tc.id, tc.id, tc.id, tc.delivery)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	for i, want := range []int64{1, 1, 0} {
		stats, cleanupErr := repo.RunRetentionCleanupBatch(t.Context(), time.Now(), time.Now(), 1)
		if cleanupErr != nil {
			t.Fatal(cleanupErr)
		}
		if stats.DeletedCoverDownloads != want {
			t.Fatalf("batch %d deleted %d downloads, want %d", i, stats.DeletedCoverDownloads, want)
		}
	}
	rows, err := repo.DB().Query(`SELECT id FROM cover_downloads ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ids, ","); got != "canceled,failure,queued,running,success" {
		t.Fatalf("remaining downloads = %s", got)
	}
	for table, want := range map[string]int{"cover_candidates": 1, "cover_deliveries": 3} {
		var count int
		if err := repo.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
}

func TestCoverQueueKeepsRowIDOrderAfterRetention(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	insert := func(id, status string) {
		t.Helper()
		if _, err := repo.DB().Exec(`
			INSERT INTO cover_downloads(id,work_code,version,url,backup_url,locale,refresh,status)
			VALUES(?,?,1,'url','','ja',0,?)
		`, id, id, status); err != nil {
			t.Fatal(err)
		}
	}
	insert("z", "queued")
	insert("middle", "done")
	insert("a", "queued")
	insert("tail", "done")
	if _, err := repo.RunRetentionCleanupBatch(t.Context(), time.Now(), time.Now(), 500); err != nil {
		t.Fatal(err)
	}
	insert("new", "queued")
	for _, want := range []string{"z", "a", "new"} {
		job, err := repo.ClaimCover(t.Context())
		if err != nil || job == nil || job.ID != want {
			t.Fatalf("claim=%+v err=%v want=%s", job, err, want)
		}
	}
	if _, err := repo.DB().Exec(`UPDATE cover_downloads SET status='queued' WHERE id='z'`); err != nil {
		t.Fatal(err)
	}
	insert("later", "queued")
	for _, want := range []string{"z", "later"} {
		job, err := repo.ClaimCover(t.Context())
		if err != nil || job == nil || job.ID != want {
			t.Fatalf("claim=%+v err=%v want=%s", job, err, want)
		}
	}
	for query, index := range map[string]string{
		`SELECT id FROM cover_downloads WHERE status='queued' ORDER BY rowid LIMIT 1`: "cover_downloads_queue",
		`SELECT 1 FROM cover_deliveries WHERE job_id='job'`:                           "cover_deliveries_job",
		`SELECT j.id FROM cover_downloads j WHERE j.status='done' AND NOT EXISTS (
			SELECT 1 FROM cover_deliveries d WHERE d.job_id=j.id) LIMIT 500`: "cover_deliveries_job",
	} {
		rows, err := repo.DB().Query("EXPLAIN QUERY PLAN " + query)
		if err != nil {
			t.Fatal(err)
		}
		var plan []string
		for rows.Next() {
			var a, b, c int
			var detail string
			if err := rows.Scan(&a, &b, &c, &detail); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		joined := strings.Join(plan, " ")
		if !strings.Contains(joined, index) || strings.Contains(joined, "TEMP B-TREE") {
			t.Fatalf("%s plan: %v", query, plan)
		}
	}
}

func TestCoverQueueSharesDownloadsAndRecoversWithoutRevivingCanceledRequests(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ctx := t.Context()
	if _, err = repo.DB().
		Exec(`INSERT INTO worksets(id) VALUES('set'); INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id) VALUES('plan-a','/music','s','set'),('plan-b','/music','s','set'); INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('exec-a','set','plan-a'),('exec-b','set','plan-b');`); err != nil {
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
	stats, err := repo.RunRetentionCleanupBatch(ctx, time.Now(), time.Now(), 500)
	if err != nil || stats.DeletedCoverDownloads != 0 {
		t.Fatalf("shared download was reclaimed: %+v %v", stats, err)
	}
	if err := repo.FinishCover(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if destinations, err := repo.CoverDestinations(ctx, job.ID); err != nil || len(destinations) != 2 {
		t.Fatalf("shared delivery lost: %+v %v", destinations, err)
	}
}
