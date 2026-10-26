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
	_, repo, dl, req, folders := deliveryFixture(t)
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
	svc := selectedRefreshService(t, repo, dl, func(ctx context.Context, base string, selected []string) error {
		calls++
		if base != root || !slices.Equal(selected, folders[:1]) {
			t.Fatalf("wrong scope: %s %v", base, selected)
		}
		_, err := pipeline.ScanFoldersCtx(ctx, selected, base, 6)
		return err
	})
	if err := svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	svc.ScheduleRefresh("exec", root)
	assertRefresh(t, svc, false, nil) // Metadata still queued, cover still pending.
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
		if count, err := repo.RootExistsInInventory(path); err != nil ||
			!count {
			t.Fatalf("inventory %s: %v %v", path, count, err)
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
	store := &refreshReportStore{reports: map[string]coverdelivery.Report{
		"exec": {
			Counts: map[string]int{"failed": 1, "canceled": 1},
			Items: []coverdelivery.Destination{{
				Target: coverdelivery.Target{WorkCode: "RJ00000001", RelPath: "copy"},
				Status: "failed", ErrorCode: "INVENTORY_REFRESH_FAILED",
			}},
		},
		"other": {Counts: map[string]int{"running": 1}},
	}}
	calls := 0
	svc, err := coverdelivery.New(coverdelivery.Dependencies{
		Store: store, Downloader: &download{}, Files: filesystem.CoverFiles{},
		Acquire: func() (func(), error) { return func() {}, nil },
		RefreshSelected: func(_ context.Context, root string, folders []string) error {
			calls++
			if root != "/music" || !slices.Equal(folders, []string{"/music/copy"}) {
				t.Fatalf("wrong repair scope: %s %v", root, folders)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.ScheduleRefresh("exec", "/music")
	assertRefresh(t, svc, true, nil)
	if calls != 1 || store.reports["other"].Counts["running"] != 1 {
		t.Fatal("repair waited for or changed another execution's shared download")
	}
}

func TestSelectedRefreshFailureAndEmptyScopeDoNotChangeMetadata(t *testing.T) {
	_, repo, dl, req, _ := deliveryFixture(t)
	scanErr := errors.New("walk failed")
	calls := 0
	svc := selectedRefreshService(t, repo, dl, func(context.Context, string, []string) error {
		calls++
		return scanErr
	})
	finishMetadata(t, repo)
	svc.ScheduleRefresh("empty", req.Targets[0].LibraryRoot)
	assertRefresh(t, svc, true, nil)
	if err := svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	before, reportErr := svc.Report(t.Context(), "exec")
	if reportErr != nil || before.Counts["failed"] != 2 {
		t.Fatalf("cover failures: %+v %v", before, reportErr)
	}
	svc.ScheduleRefresh("exec", req.Targets[0].LibraryRoot)
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
	after, readErr := svc.Report(t.Context(), "exec")
	if readErr != nil || !slices.Equal(before.Items, after.Items) {
		t.Fatalf("repair changed cover results: %+v %v", after, readErr)
	}
	if err := svc.Retry(t.Context(), "exec"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertRefresh(t, svc, false, nil) // Explicit retries never register another repair.
}

func selectedRefreshService(
	t *testing.T, repo *sqlite.Repository, dl *download,
	refresh func(context.Context, string, []string) error,
) *coverdelivery.Service {
	t.Helper()
	svc, err := coverdelivery.New(coverdelivery.Dependencies{
		Store: repo, Downloader: dl, Files: filesystem.CoverFiles{},
		Acquire: admission.NewGate(repo.HasActiveSession).BeginManual, RefreshSelected: refresh,
		SyncInventory: func(context.Context, coverdelivery.Target, coverdelivery.Publication) error {
			return errors.New("inventory unavailable")
		},
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

type refreshReportStore struct {
	coverdelivery.Store

	reports map[string]coverdelivery.Report
}

func (s *refreshReportStore) CoverReport(_ context.Context, id string) (coverdelivery.Report, error) {
	return s.reports[id], nil
}

func TestSelectedRefreshSuccessfulCoversDoNotAcquire(t *testing.T) {
	store := &refreshReportStore{reports: map[string]coverdelivery.Report{
		"exec": {Counts: map[string]int{"succeeded": 1, "skipped": 1}},
	}}
	svc, err := coverdelivery.New(coverdelivery.Dependencies{
		Store: store, Downloader: &download{}, Files: filesystem.CoverFiles{},
		Acquire: func() (func(), error) {
			t.Fatal("successful covers acquired file-operation admission")
			return nil, nil
		},
		RefreshSelected: func(context.Context, string, []string) error {
			t.Fatal("successful covers scanned inventory")
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.ScheduleRefresh("exec", "/music")
	assertRefresh(t, svc, true, nil)
	assertRefresh(t, svc, false, nil)
}

func TestSelectedRefreshWaitsForAllDemandsAndDeduplicatesDirectories(t *testing.T) {
	store := &refreshReportStore{reports: map[string]coverdelivery.Report{
		"exec": {
			Counts: map[string]int{"failed": 3, "pending": 1},
			Items: []coverdelivery.Destination{
				{
					Target:    coverdelivery.Target{WorkCode: "RJ00000001", RelPath: "copy-b"},
					ErrorCode: "INVENTORY_REFRESH_FAILED",
					Published: "cover.png",
				},
				{
					Target:    coverdelivery.Target{WorkCode: "RJ00000001", RelPath: "copy-a"},
					ErrorCode: "INVENTORY_REFRESH_FAILED",
				},
				{
					Target:    coverdelivery.Target{WorkCode: "RJ00000001", RelPath: "copy-b"},
					ErrorCode: "INVENTORY_REFRESH_FAILED",
				},
			},
		},
	}}
	busy := true
	acquisitions, scans := 0, 0
	svc, err := coverdelivery.New(coverdelivery.Dependencies{
		Store: store, Downloader: &download{}, Files: filesystem.CoverFiles{},
		Acquire: func() (func(), error) {
			acquisitions++
			if busy {
				return nil, &admission.BusyError{Reason: "metadata execution still active"}
			}
			return func() {}, nil
		},
		RefreshSelected: func(_ context.Context, root string, folders []string) error {
			scans++
			if root != "/music" || !slices.Equal(folders, []string{"/music/copy-a", "/music/copy-b"}) {
				t.Fatalf("wrong repair scope: %s %v", root, folders)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.ScheduleRefresh("exec", "/music")
	for _, status := range []string{"pending", "running"} {
		store.reports["exec"].Counts["pending"] = 0
		store.reports["exec"].Counts[status] = 1
		assertRefresh(t, svc, false, nil)
		store.reports["exec"].Counts[status] = 0
	}
	if acquisitions != 0 {
		t.Fatal("repair requested admission before all demands were terminal")
	}
	assertRefresh(t, svc, false, nil) // All demands ended, but execution admission is busy.
	busy = false
	assertRefresh(t, svc, true, nil)
	assertRefresh(t, svc, false, nil)
	if scans != 1 {
		t.Fatalf("repair scans: %d", scans)
	}
}

func TestSelectedRefreshOtherFailuresDoNotAcquire(t *testing.T) {
	for _, tc := range []struct {
		name, status, code string
	}{
		{"no demands", "", ""},
		{"download failed", "failed", "COVER_DOWNLOAD_FAILED"},
		{"publication failed", "failed", "COVER_PUBLICATION_FAILED"},
		{"canceled", "canceled", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := coverdelivery.Report{Counts: map[string]int{}}
			if tc.status != "" {
				report.Counts[tc.status] = 1
				report.Items = []coverdelivery.Destination{{
					Target: coverdelivery.Target{RelPath: "copy"}, Status: tc.status, ErrorCode: tc.code,
				}}
			}
			store := &refreshReportStore{reports: map[string]coverdelivery.Report{"exec": report}}
			svc, err := coverdelivery.New(coverdelivery.Dependencies{
				Store: store, Downloader: &download{}, Files: filesystem.CoverFiles{},
				Acquire: func() (func(), error) {
					t.Fatal("cover without inventory failure requested admission")
					return nil, nil
				},
				RefreshSelected: func(context.Context, string, []string) error {
					t.Fatal("cover without inventory failure scanned inventory")
					return nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			svc.ScheduleRefresh("exec", "/music")
			assertRefresh(t, svc, true, nil)
			assertRefresh(t, svc, false, nil)
		})
	}
}

func TestSelectedRefreshRepairsReusedCoverInventoryFailure(t *testing.T) {
	_, repo, dl, req, folders := deliveryFixture(t)
	req.Targets = req.Targets[:1]
	if err := os.WriteFile(filepath.Join(folders[0], "cover.png"), dl.data, 0600); err != nil {
		t.Fatal(err)
	}
	finishMetadata(t, repo)
	scans := 0
	svc := selectedRefreshService(t, repo, dl, func(_ context.Context, root string, selected []string) error {
		scans++
		if root != req.Targets[0].LibraryRoot || !slices.Equal(selected, folders[:1]) {
			t.Fatalf("wrong reused-cover repair scope: %s %v", root, selected)
		}
		return nil
	})
	if err := svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	svc.ScheduleRefresh("exec", req.Targets[0].LibraryRoot)
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(t.Context(), "exec")
	if err != nil || report.Counts["failed"] != 1 || len(report.Items) != 1 ||
		report.Items[0].ErrorCode != "INVENTORY_REFRESH_FAILED" || dl.calls != 0 {
		t.Fatalf("existing cover was not reused: %+v %v downloads=%d", report, err, dl.calls)
	}
	assertRefresh(t, svc, true, nil)
	assertRefresh(t, svc, false, nil)
	if scans != 1 {
		t.Fatalf("reused-cover repair scans: %d", scans)
	}
}
