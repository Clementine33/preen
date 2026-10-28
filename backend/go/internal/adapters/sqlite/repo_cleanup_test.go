package sqlite //nolint:testpackage // SQLite fault injection proves atomic deletion and summary commits.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

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
		Counts:     make(map[cleanup.Category]cleanup.Count),
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
	run := cleanup.Run{ID: "run", Status: "running", Counts: make(map[cleanup.Category]cleanup.Count)}
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

func TestManualCleanupIgnoresRetentionWhileAutomaticKeepsIt(t *testing.T) {
	repo := newTestRepository(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ id, status string }{{"scan-recent", "completed"}, {"scan-active", "running"}} {
		if err := repo.CreateScanSession(&inventory.ScanSession{
			SessionID: tc.id, RootPath: "/music", Kind: "full", Status: tc.status, StartedAt: now.Add(-time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	ws := seedRetentionWorkset(t, repo)
	seedFinishedGeneration(t, repo, ws, "gen-recent", now.Add(-time.Hour))
	seedGeneration(t, repo, "gen-active", ws)

	stats, err := repo.RunRetentionCleanupBatch(ctx, now.Add(-2*24*time.Hour), now.Add(-5*24*time.Hour), 500)
	if err != nil || stats.DeletedScanSessions != 0 || stats.DeletedGenerations != 0 {
		t.Fatalf("automatic retention %+v %v", stats, err)
	}

	run := cleanup.Run{
		ID: "manual", Status: "running",
		Categories: []cleanup.Category{cleanup.ScanSessions, cleanup.PlanGenerations},
		Counts:     make(map[cleanup.Category]cleanup.Count),
	}
	if createErr := repo.CreateCleanupRun(ctx, run); createErr != nil {
		t.Fatal(createErr)
	}
	for _, category := range run.Categories {
		ids, candidateErr := repo.CleanupCandidates(ctx, category, now, nil)
		if candidateErr != nil || len(ids) != 1 {
			t.Fatalf("%s candidates %v %v", category, ids, candidateErr)
		}
		if batchErr := repo.CleanupBatch(ctx, run.ID, category, now, ids); batchErr != nil {
			t.Fatal(batchErr)
		}
	}
	// Only the unfinished records remain.
	if countRows(t, repo, "scan_sessions") != 1 || countRows(t, repo, "plan_generations") != 1 {
		t.Fatal("manual cleanup must delete recent terminal records and keep active ones")
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
	run := cleanup.Run{ID: "run", Status: "running", Counts: make(map[cleanup.Category]cleanup.Count)}
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

func TestGenerationRecordCleanupKeepsRecordsWithAnEntry(t *testing.T) {
	repo := newTestRepository(t)
	ctx := t.Context()
	now := time.Now().UTC()
	record := `INSERT INTO generation_records(path,codec,encoder,encoder_version,bitrate_kbps,mode,size,mtime,content_sha256,created_at)
 VALUES (?,'aac','enc','1',256,'cbr',1,1,'sum','2026-10-01T00:00:00Z')`
	for _, path := range []string{"/music/kept.m4a", "/music/moved.m4a", "/music/back.m4a"} {
		if _, err := repo.db.Exec(record, path); err != nil {
			t.Fatal(err)
		}
	}
	inventoryfixture.SeedInventory(t, repo, inventoryfixture.Row{Path: "/music/kept.m4a", RootPath: "/music"})
	ids, err := repo.CleanupCandidates(ctx, cleanup.GenerationRecords, now, nil)
	slices.Sort(ids)
	if err != nil || !slices.Equal(ids, []string{"/music/back.m4a", "/music/moved.m4a"}) {
		t.Fatalf("candidates %v %v", ids, err)
	}
	// A scan finds one file again before the batch: the recheck keeps its record.
	inventoryfixture.SeedInventory(t, repo, inventoryfixture.Row{Path: "/music/back.m4a", RootPath: "/music"})
	run := cleanup.Run{ID: "run", Status: "running", Counts: make(map[cleanup.Category]cleanup.Count)}
	if createErr := repo.CreateCleanupRun(ctx, run); createErr != nil {
		t.Fatal(createErr)
	}
	if batchErr := repo.CleanupBatch(ctx, run.ID, cleanup.GenerationRecords, now, ids); batchErr != nil {
		t.Fatal(batchErr)
	}
	saved, err := repo.ReadCleanupRun(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Counts[cleanup.GenerationRecords] != (cleanup.Count{Checked: 2, Deleted: 1, Skipped: 1}) ||
		countRows(t, repo, "generation_records") != 2 {
		t.Fatalf("rechecked %+v", saved)
	}

	// Automatic maintenance removes an orphan the same way.
	if err = NewScanStaging(
		repo,
	).WriteStagingEntries("remove-back", []inventory.StagingEntry{{SessionID: "remove-back", Path: "/music/kept.m4a", RootPath: "/music", ParentPath: "/music", Name: "kept.m4a"}}); err != nil {
		t.Fatal(err)
	}
	if err = repo.MergeStagingSimple("remove-back", "/music"); err != nil {
		t.Fatal(err)
	}
	stats, err := repo.RunRetentionCleanupBatch(ctx, now, now, 0)
	if err != nil || stats.DeletedGenerationRecords != 1 || countRows(t, repo, "generation_records") != 1 {
		t.Fatalf("automatic %+v %v", stats, err)
	}
}
