package coverdelivery_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

func TestSelectedRefreshWaitsForMetadataAndCoversThenMergesOnce(t *testing.T) {
	_, repo, dl, req, folders, stage := deliveryFixture(t)
	root := req.Targets[0].LibraryRoot
	req.Targets = req.Targets[:1]
	newFile := filepath.ToSlash(filepath.Join(folders[0], "new.txt"))
	untouched := filepath.ToSlash(filepath.Join(folders[1], "unselected.txt"))
	if err := os.WriteFile(newFile, []byte("observed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := repo.SyncObservedInventory(
		root,
		nil,
		[]inventory.InventoryFile{{Path: untouched, Size: 17}},
		nil,
	); err != nil {
		t.Fatal(err)
	}
	pipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo), filesystem.WalkRootEntriesParallel, filesystem.WalkFolderEntries, nil,
	)
	calls := 0
	svc := selectedRefreshService(t, repo, dl, stage, func(ctx context.Context, base string, selected []string) error {
		calls++
		if base != root || !slices.Equal(selected, folders[:1]) {
			t.Fatalf("wrong scope: %s %v", base, selected)
		}
		_, err := pipeline.ScanFoldersCtx(ctx, selected, base, 6)
		return err
	})
	svc.ScheduleRefresh("exec", root, folders[:1])
	assertRefresh(t, svc, false, nil) // Metadata still queued, no cover rows yet.
	if err := svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	finishMetadata(t, repo)
	assertRefresh(t, svc, false, nil) // Metadata terminal, cover still pending.
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertRefresh(t, svc, true, nil)
	assertRefresh(t, svc, false, nil)
	if calls != 1 {
		t.Fatalf("refresh calls: %d", calls)
	}
	for _, path := range []string{newFile, untouched, filepath.ToSlash(filepath.Join(folders[0], "cover.png"))} {
		var count int
		if err := repo.DB().
			QueryRow(`SELECT COUNT(*) FROM entries WHERE path=?`, path).
			Scan(&count); err != nil ||
			count != 1 {
			t.Fatalf("inventory %s: %d %v", path, count, err)
		}
	}
	var count int
	if err := repo.DB().
		QueryRow(`SELECT COUNT(*) FROM scan_sessions WHERE kind='selected'`).
		Scan(&count); err != nil ||
		count != 1 {
		t.Fatalf("selected scan sessions: %d %v", count, err)
	}
}

func TestSelectedRefreshDoesNotWaitForOtherExecutionsSharedJob(t *testing.T) {
	_, repo, dl, req, folders, stage := deliveryFixture(t)
	if _, err := repo.DB().Exec(`INSERT INTO plan_executions(execution_id,workset_id,plan_id,status)
		VALUES('other','set','other-plan','succeeded')`); err != nil {
		t.Fatal(err)
	}
	calls := 0
	svc := selectedRefreshService(t, repo, dl, stage, func(context.Context, string, []string) error {
		calls++
		return nil
	})
	if err := svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	other := req
	other.ExecutionID = "other"
	if err := svc.Enqueue(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	svc.ScheduleRefresh("exec", req.Targets[0].LibraryRoot, folders)
	finishMetadata(t, repo)
	if err := svc.Cancel(t.Context(), "exec"); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(t.Context(), "exec")
	if err != nil || report.Counts["canceled"] != 2 || report.Downloads["queued"] != 1 {
		t.Fatalf("shared job: %+v %v", report, err)
	}
	assertRefresh(t, svc, true, nil)
	if calls != 1 {
		t.Fatal("canceled demands prevented completion")
	}
}

func TestSelectedRefreshFailureAndEmptyScopeDoNotChangeMetadata(t *testing.T) {
	_, repo, dl, req, folders, stage := deliveryFixture(t)
	scanErr := errors.New("walk failed")
	calls := 0
	svc := selectedRefreshService(t, repo, dl, stage, func(context.Context, string, []string) error {
		calls++
		return scanErr
	})
	finishMetadata(t, repo)
	svc.ScheduleRefresh("empty", req.Targets[0].LibraryRoot, nil)
	assertRefresh(t, svc, false, nil)
	svc.ScheduleRefresh("exec", req.Targets[0].LibraryRoot, folders)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if worked, err := svc.RefreshNext(ctx); worked || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled refresh: %v %v", worked, err)
	}
	assertRefresh(t, svc, true, scanErr)
	assertRefresh(t, svc, false, nil)
	var status string
	if err := repo.DB().
		QueryRow(`SELECT status FROM plan_executions WHERE execution_id='exec'`).
		Scan(&status); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || status != "succeeded" {
		t.Fatalf("refresh changed execution: %s, calls %d", status, calls)
	}
}

func selectedRefreshService(
	t *testing.T, repo *sqlite.Repository, dl *download, stage string,
	refresh func(context.Context, string, []string) error,
) *coverdelivery.Service {
	t.Helper()
	svc, err := coverdelivery.New(coverdelivery.Dependencies{
		Store: repo, Downloader: dl, Files: filesystem.CoverFiles{}, Staging: coverStaging(t, stage),
		Acquire: admission.NewGate(repo.HasActiveSession).BeginManual, RefreshSelected: refresh,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func finishMetadata(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	if _, err := repo.DB().Exec(`UPDATE plan_executions SET status='succeeded' WHERE execution_id='exec'`); err != nil {
		t.Fatal(err)
	}
}

func assertRefresh(t *testing.T, svc *coverdelivery.Service, want bool, wantErr error) {
	t.Helper()
	if worked, err := svc.RefreshNext(t.Context()); worked != want || !errors.Is(err, wantErr) {
		t.Fatalf("refresh: worked=%v err=%v, want %v %v", worked, err, want, wantErr)
	}
}
