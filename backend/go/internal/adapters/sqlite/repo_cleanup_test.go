package sqlite //nolint:testpackage // SQLite fault injection proves atomic deletion and summary commits.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/cleanup"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

func TestCleanupBatchCommitsDeletionAndCountTogether(t *testing.T) {
	repo := newTestRepository(t)
	ctx := t.Context()
	now := time.Now().UTC()
	for _, id := range []string{"first", "second"} {
		if err := repo.CreateScanSession(
			&inventory.ScanSession{
				SessionID: id,
				RootPath:  "/music",
				Kind:      "full",
				Status:    "completed",
				StartedAt: now.Add(-48 * time.Hour),
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	run := cleanup.Run{
		ID:         "run",
		Status:     "running",
		Categories: []cleanup.Category{cleanup.ScanSessions},
		StartedAt:  now,
		Counts:     map[cleanup.Category]cleanup.Count{},
	}
	if createErr := repo.CreateCleanupRun(ctx, run); createErr != nil {
		t.Fatal(createErr)
	}
	ids, err := repo.CleanupCandidates(ctx, cleanup.ScanSessions, now.Add(-24*time.Hour), nil)
	if err != nil || len(ids) != 2 {
		t.Fatalf("candidates %v %v", ids, err)
	}
	if batchErr := repo.CleanupBatch(
		ctx,
		run.ID,
		cleanup.ScanSessions,
		now.Add(-24*time.Hour),
		ids[:1],
	); batchErr != nil {
		t.Fatal(batchErr)
	}
	if _, triggerErr := repo.db.Exec(
		"CREATE TRIGGER reject_progress BEFORE UPDATE ON cleanup_run BEGIN SELECT RAISE(ABORT,'injected'); END",
	); triggerErr != nil {
		t.Fatal(triggerErr)
	}
	if batchErr := repo.CleanupBatch(
		ctx,
		run.ID,
		cleanup.ScanSessions,
		now.Add(-24*time.Hour),
		ids[1:],
	); batchErr == nil {
		t.Fatal("expected summary failure")
	}
	saved, err := repo.ReadCleanupRun(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Counts[cleanup.ScanSessions].Deleted != 1 || countRows(t, repo, "scan_sessions") != 1 {
		t.Fatalf("atomic summary: %+v", saved)
	}
}
func TestCleanupStopCannotBeOverwrittenAndRecoveryInterrupts(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()
	run := cleanup.Run{ID: "run", Status: "running", Counts: map[cleanup.Category]cleanup.Count{}}
	if createErr := repo.CreateCleanupRun(ctx, run); createErr != nil {
		t.Fatal(createErr)
	}
	if _, err := repo.StopCleanupRun(ctx, "wrong"); !errors.Is(err, cleanup.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := repo.StopCleanupRun(ctx, "run"); err != nil {
		t.Fatal(err)
	}
	if err := repo.FinishCleanupRun(ctx, "run", "succeeded", ""); err != nil {
		t.Fatal(err)
	}
	saved, _ := repo.ReadCleanupRun(ctx)
	if saved.Status != "stopped" {
		t.Fatalf("status %s", saved.Status)
	}
	run.ID = "next"
	if createErr := repo.CreateCleanupRun(ctx, run); createErr != nil {
		t.Fatal(createErr)
	}
	if err := repo.InterruptCleanupRun(ctx); err != nil {
		t.Fatal(err)
	}
	saved, _ = repo.ReadCleanupRun(ctx)
	if saved.Status != "interrupted" {
		t.Fatalf("status %s", saved.Status)
	}
}

func TestManualAndAutomaticCleanupShareRetentionAndSelection(t *testing.T) {
	repo := newTestRepository(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	opts := cleanup.Options{ScanRetention: 2 * 24 * time.Hour, GenerationRetention: 5 * 24 * time.Hour}
	for _, tc := range []struct {
		id, status string
		age        time.Duration
	}{
		{"scan-old", "completed", 3 * 24 * time.Hour}, {"scan-boundary", "completed", 2 * 24 * time.Hour},
		{"scan-active", "running", 10 * 24 * time.Hour}, {"scan-failed", "failed", 3 * 24 * time.Hour},
	} {
		if err := repo.CreateScanSession(
			&inventory.ScanSession{
				SessionID: tc.id,
				RootPath:  "/music",
				Kind:      "full",
				Status:    tc.status,
				StartedAt: now.Add(-tc.age),
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	ws := seedRetentionWorkset(t, repo)
	seedFinishedGeneration(t, repo, ws, "gen-old", now.Add(-6*24*time.Hour))
	seedFinishedGeneration(t, repo, ws, "gen-boundary", now.Add(-5*24*time.Hour))
	seedGeneration(t, repo, "gen-active", ws)
	if _, err := repo.db.Exec(
		`INSERT INTO cover_downloads(id,work_code,version,url,backup_url,locale,refresh,status) VALUES ('orphan','RJ00000001',1,'url','','ja',0,'done')`,
	); err != nil {
		t.Fatal(err)
	}
	run := cleanup.Run{
		ID:         "selected",
		Status:     "running",
		Categories: []cleanup.Category{cleanup.ScanSessions},
		Counts:     map[cleanup.Category]cleanup.Count{},
	}
	if createErr := repo.CreateCleanupRun(ctx, run); createErr != nil {
		t.Fatal(createErr)
	}
	ids, err := repo.CleanupCandidates(ctx, cleanup.ScanSessions, opts.Cutoff(cleanup.ScanSessions, now), nil)
	if err != nil || len(ids) != 2 {
		t.Fatalf("scan candidates %v %v", ids, err)
	}
	if batchErr := repo.CleanupBatch(
		ctx,
		run.ID,
		cleanup.ScanSessions,
		opts.Cutoff(cleanup.ScanSessions, now),
		ids,
	); batchErr != nil {
		t.Fatal(batchErr)
	}
	if countRows(t, repo, "plan_generations") != 3 || countRows(t, repo, "cover_downloads") != 1 {
		t.Fatal("unselected categories changed")
	}
	ids, err = repo.CleanupCandidates(ctx, cleanup.PlanGenerations, opts.Cutoff(cleanup.PlanGenerations, now), nil)
	if err != nil || len(ids) != 1 {
		t.Fatalf("generation candidates %v %v", ids, err)
	}
	if batchErr := repo.CleanupBatch(
		ctx,
		run.ID,
		cleanup.PlanGenerations,
		opts.Cutoff(cleanup.PlanGenerations, now),
		ids,
	); batchErr != nil {
		t.Fatal(batchErr)
	}
	if _, updateErr := repo.db.Exec(
		"UPDATE plan_generations SET finished_at=? WHERE generation_id='gen-boundary'",
		now.Add(-6*24*time.Hour).Format(timeFormat),
	); updateErr != nil {
		t.Fatal(updateErr)
	}
	stats, err := repo.RunRetentionCleanupBatch(
		ctx,
		opts.Cutoff(cleanup.ScanSessions, now),
		opts.Cutoff(cleanup.PlanGenerations, now),
		500,
	)
	if err != nil || stats.DeletedScanSessions != 0 || stats.DeletedGenerations != 1 ||
		stats.DeletedCoverDownloads != 1 {
		t.Fatalf("shared windows %+v %v", stats, err)
	}
}
func TestCleanupCandidateSnapshotRechecksProtectionAndSkipsNewRows(t *testing.T) {
	repo := newTestRepository(t)
	ctx := t.Context()
	now := time.Now().UTC()
	_, err := repo.db.Exec(`
 INSERT INTO worksets(id) VALUES('set');
 INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id) VALUES('plan','/music','s','set');
 INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('exec','set','plan');
 INSERT INTO cover_downloads(id,work_code,version,url,backup_url,locale,refresh,status) VALUES ('before','before',1,'url','','ja',0,'done');`)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := repo.CleanupCandidates(ctx, cleanup.CoverDownloads, now, nil)
	if err != nil || len(ids) != 1 {
		t.Fatalf("candidates %v %v", ids, err)
	}
	_, err = repo.db.Exec(`
 INSERT INTO cover_deliveries(id,job_id,execution_id,copy_id,target_json,refresh,status) VALUES('delivery','before','exec','copy','{}',0,'canceled');
 INSERT INTO cover_downloads(id,work_code,version,url,backup_url,locale,refresh,status) VALUES ('after','after',1,'url','','ja',0,'done');`)
	if err != nil {
		t.Fatal(err)
	}
	run := cleanup.Run{ID: "run", Status: "running", Counts: map[cleanup.Category]cleanup.Count{}}
	if createErr := repo.CreateCleanupRun(ctx, run); createErr != nil {
		t.Fatal(createErr)
	}
	if batchErr := repo.CleanupBatch(ctx, run.ID, cleanup.CoverDownloads, now, ids); batchErr != nil {
		t.Fatal(batchErr)
	}
	saved, err := repo.ReadCleanupRun(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Counts[cleanup.CoverDownloads] != (cleanup.Count{Checked: 1, Skipped: 1}) ||
		countRows(t, repo, "cover_downloads") != 2 {
		t.Fatalf("rechecked %+v", saved)
	}
}

func TestManualDownloadCleanupKeepsEveryDeliveryStatus(t *testing.T) {
	repo := newTestRepository(t)
	ctx := t.Context()
	_, err := repo.db.Exec(`
 INSERT INTO worksets(id) VALUES('set');
 INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id) VALUES('plan','/music','s','set');
 INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('exec','set','plan');`)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"pending", "running", "failed", "canceled", "succeeded", "done"} {
		if _, insertErr := repo.db.Exec(
			`INSERT INTO cover_downloads(id,work_code,version,url,backup_url,locale,refresh,status) VALUES (?,?,1,'url','','ja',0,'done')`,
			status,
			status,
		); insertErr != nil {
			t.Fatal(insertErr)
		}
		if _, insertErr := repo.db.Exec(
			`INSERT INTO cover_deliveries(id,job_id,execution_id,copy_id,target_json,refresh,status) VALUES(?,?,'exec',?,'{}',0,?)`,
			status,
			status,
			status,
			status,
		); insertErr != nil {
			t.Fatal(insertErr)
		}
	}
	if _, insertErr := repo.db.Exec(
		`INSERT INTO cover_downloads(id,work_code,version,url,backup_url,locale,refresh,status) VALUES ('orphan','orphan',1,'url','','ja',0,'done')`,
	); insertErr != nil {
		t.Fatal(insertErr)
	}
	ids, err := repo.CleanupCandidates(ctx, cleanup.CoverDownloads, time.Now(), nil)
	if err != nil || len(ids) != 1 || ids[0] != "orphan" {
		t.Fatalf("candidates %v %v", ids, err)
	}
	run := cleanup.Run{ID: "run", Status: "running", Counts: map[cleanup.Category]cleanup.Count{}}
	if createErr := repo.CreateCleanupRun(ctx, run); createErr != nil {
		t.Fatal(createErr)
	}
	if batchErr := repo.CleanupBatch(ctx, run.ID, cleanup.CoverDownloads, time.Now(), ids); batchErr != nil {
		t.Fatal(batchErr)
	}
	if countRows(t, repo, "cover_downloads") != 6 || countRows(t, repo, "cover_deliveries") != 6 {
		t.Fatal("referenced downloads changed")
	}
}
func TestCanceledCleanupBatchRollsBackWithoutProgress(t *testing.T) {
	repo := newTestRepository(t)
	ctx := t.Context()
	now := time.Now().UTC()
	if err := repo.CreateScanSession(
		&inventory.ScanSession{
			SessionID: "scan",
			RootPath:  "/music",
			Kind:      "full",
			Status:    "completed",
			StartedAt: now.Add(-48 * time.Hour),
		},
	); err != nil {
		t.Fatal(err)
	}
	run := cleanup.Run{ID: "run", Status: "running", Counts: map[cleanup.Category]cleanup.Count{}}
	if err := repo.CreateCleanupRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := repo.CleanupBatch(
		canceled,
		run.ID,
		cleanup.ScanSessions,
		now,
		[]string{"scan"},
	); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf("cancel: %v", err)
	}
	saved, err := repo.ReadCleanupRun(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if countRows(t, repo, "scan_sessions") != 1 || saved.Counts[cleanup.ScanSessions].Deleted != 0 {
		t.Fatal("canceled transaction committed")
	}
	if _, err := repo.CleanupCandidates(canceled, cleanup.ScanSessions, now, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("discovery cancel: %v", err)
	}
}
