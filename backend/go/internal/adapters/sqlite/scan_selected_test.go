package sqlite_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

func TestSelectedScanPreservesScopeMarkersNeighborsAndMediaFacts(t *testing.T) {
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	root := filepath.ToSlash(t.TempDir())
	scope := root + "/a"
	if mkdirErr := os.MkdirAll(filepath.FromSlash(scope+"/child"), 0o755); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	file := scope + "/keep.mp3"
	if writeErr := os.WriteFile(filepath.FromSlash(file), []byte("audio"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	info, err := os.Stat(filepath.FromSlash(file))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{scope, file, scope + "/deleted.mp3", root + "/a2/unselected.mp3", root + "/other/unselected.mp3"} {
		inventoryfixture.SeedInventory(
			t,
			repo,
			inventoryfixture.Row{
				Path:        path,
				RootPath:    root,
				IsDir:       path == scope,
				IsWorkRoot:  path == scope,
				Size:        5,
				Mtime:       info.ModTime().Unix(),
				Format:      "mpeg",
				Bitrate:     new(int32(320000)),
				DurationSec: new(float64(12)),
				ContentRev:  7,
			},
		)
	}
	seedSelectedBinding(t, repo, root)
	pipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo),
		filesystem.WalkRootEntriesParallel,
		filesystem.WalkFolderEntries,
		nil,
	)
	if _, err := pipeline.ScanFoldersCtx(t.Context(), []string{scope, scope + "/child", scope}, root, 32); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{scope, root + "/a2/unselected.mp3", root + "/other/unselected.mp3"} {
		found, readErr := repo.RootExistsInInventory(path)
		if readErr != nil || !found {
			t.Fatalf("preserved %s: found=%v err=%v", path, found, readErr)
		}
	}
	var copyID string
	if bindErr := repo.DB().
		QueryRow("SELECT id FROM work_copies WHERE library_id = 'selected-library' AND rel_path = 'a'").
		Scan(&copyID); bindErr != nil ||
		copyID != "selected-copy" {
		t.Fatalf("binding changed: %s, %v", copyID, bindErr)
	}

	deleted, readErr := repo.RootExistsInInventory(scope + "/deleted.mp3")
	if readErr != nil || deleted {
		t.Fatalf("stale row: found=%v err=%v", deleted, readErr)
	}
	entry := inventoryEntry(t, repo, file)
	if entry.ContentRev != 7 || entry.Bitrate == nil || *entry.Bitrate != 320000 || entry.DurationSec == nil ||
		*entry.DurationSec != 12 {
		t.Fatalf("unchanged media facts changed: %+v", entry)
	}
	var count int
	if _, err := pipeline.ScanFoldersCtx(t.Context(), nil, root, 32); err != nil {
		t.Fatal(err)
	}
	if err := repo.DB().QueryRow("SELECT COUNT(*) FROM scan_sessions").Scan(&count); err != nil || count != 1 {
		t.Fatalf("empty scope started a session: %d, %v", count, err)
	}
}

func TestSelectedMergeRejectsUnboundedScopeAndRollsBack(t *testing.T) {
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store := sqlite.NewScanStaging(repo)
	if err := store.CreateScanSession(
		&inventory.ScanSession{
			SessionID: "selected",
			RootPath:  "/library",
			Kind:      "selected",
			Status:    "running",
			StartedAt: time.Now(),
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteStagingBatch(
		"selected",
		[]inventory.StagingEntry{
			{
				SessionID:  "selected",
				RootPath:   "/library",
				Path:       "/library/a/new.mp3",
				Name:       "new.mp3",
				ParentPath: "/library/a",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeStaging("selected", "", "/library"); err == nil {
		t.Fatal("selected session accepted by full merge")
	}
	if err := store.WriteStagingBatch(
		"selected",
		[]inventory.StagingEntry{
			{RootPath: "/library", Path: "/library/a/new.mp3", ParentPath: "/library/a", Name: "new.mp3"},
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().
		Exec(`CREATE TRIGGER reject_selected_cleanup BEFORE INSERT ON inventory_files BEGIN SELECT RAISE(ABORT, 'cleanup failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeSelectedStaging(t.Context(), "selected", "/library", []string{"/library/a"}); err == nil {
		t.Fatal("merge failure ignored")
	}
	entries, readErr := repo.ListEntriesUnderPath("/library")
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("merge did not roll back: %+v, %v", entries, readErr)
	}
	if _, err := repo.DB().Exec("DROP TRIGGER reject_selected_cleanup"); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteStagingBatch(
		"selected",
		[]inventory.StagingEntry{
			{RootPath: "/library", Path: "/library/a/new.mp3", ParentPath: "/library/a", Name: "new.mp3"},
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeSelectedStaging(t.Context(), "selected", "/library", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeSelectedStaging(t.Context(), "selected", "/library", []string{"/library/b"}); err == nil {
		t.Fatal("out-of-scope staging accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := repo.MergeSelectedStaging(ctx, "selected", "/library", []string{"/library/a"}); err == nil {
		t.Fatal("canceled merge accepted")
	}
}

func seedSelectedBinding(t *testing.T, repo *sqlite.Repository, root string) {
	t.Helper()
	if _, err := repo.DB().
		Exec("INSERT INTO libraries(id, root_path, root_path_key) VALUES('selected-library', ?, ?)", root, root); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().
		Exec("INSERT INTO works(id, collected_at, updated_at) VALUES('RJ000001', 'collected', 'updated')"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().
		Exec("INSERT INTO work_copies(id, work_id, library_id, rel_path, created_at) VALUES('selected-copy', 'RJ000001', 'selected-library', 'a', 'created')"); err != nil {
		t.Fatal(err)
	}
}

func TestSelectedMergeAcceptsMoreThanSQLiteExpressionDepthScopes(t *testing.T) {
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store := sqlite.NewScanStaging(repo)
	if sessionErr := store.CreateScanSession(
		&inventory.ScanSession{
			SessionID: "many",
			RootPath:  "/library",
			Kind:      "selected",
			Status:    "running",
			StartedAt: time.Now(),
		},
	); sessionErr != nil {
		t.Fatal(sessionErr)
	}
	folders := make([]string, 1100)
	for i := range folders {
		folders[i] = "/library/work" + strconv.Itoa(i)
	}
	if _, mergeErr := repo.MergeSelectedStaging(t.Context(), "many", "/library", folders); mergeErr != nil {
		t.Fatal(mergeErr)
	}
}

func TestSelectedMergeValidatesAllRangesBeforeWriting(t *testing.T) {
	for _, scenario := range []struct {
		name, path, root string
	}{
		{name: "multiple literal ranges"},
		{name: "nearby prefix", path: "/library/a%_日2/outside.mp3", root: "/library"},
		{name: "wrong root", path: "/library/a%_日/wrong.mp3", root: "/other"},
		{name: "scope marker", path: "/library/a%_日", root: "/library"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			store := sqlite.NewScanStaging(repo)
			if sessionErr := store.CreateScanSession(&inventory.ScanSession{
				SessionID: "ranges", RootPath: "/library", Kind: "selected", Status: "running", StartedAt: time.Now(),
			}); sessionErr != nil {
				t.Fatal(sessionErr)
			}
			entries := []inventory.StagingEntry{
				{SessionID: "ranges", RootPath: "/library", Path: "/library/a%_日/first.mp3"},
				{SessionID: "ranges", RootPath: "/library", Path: "/library/b/second.mp3"},
			}
			if scenario.path != "" {
				entries = append(
					entries,
					inventory.StagingEntry{SessionID: "ranges", RootPath: scenario.root, Path: scenario.path},
				)
			}
			if stagingErr := store.WriteStagingBatch("ranges", entries); stagingErr != nil {
				if scenario.name == "wrong root" {
					stored, readErr := repo.ListEntriesUnderPath("/library")
					if readErr != nil || len(stored) != 0 {
						t.Fatalf("invalid batch changed inventory: %+v, %v", stored, readErr)
					}
					return
				}
				t.Fatal(stagingErr)
			}
			_, err = repo.MergeSelectedStaging(t.Context(), "ranges", "/library",
				[]string{"/library/a%_日", "/library/a%_日/nested", "/library/a%_日", "/library/b"})
			want := 2
			if scenario.path != "" {
				want = 0
				if err == nil {
					t.Fatal("invalid staging was accepted")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			stored, readErr := repo.ListEntriesUnderPath("/library")
			if readErr != nil || len(stored) != want {
				t.Fatalf("inventory rows = %d, want %d: %v", len(stored), want, readErr)
			}
		})
	}
}

// staleFilesPerFolder prices each emptied folder above half the merge budget, so
// every folder takes a batch of its own.
const staleFilesPerFolder = 1001

// seedEmptiedFolders seeds folders whose stale files the next scan removes.
func seedEmptiedFolders(t *testing.T, repo *sqlite.Repository, root string, n int) []string {
	t.Helper()
	folders := make([]string, n)
	for i := range folders {
		folders[i] = root + "/f" + strconv.Itoa(i)
		if err := os.MkdirAll(filepath.FromSlash(folders[i]), 0o755); err != nil {
			t.Fatal(err)
		}
		rows := []inventoryfixture.Row{{Path: folders[i], RootPath: root, IsDir: true}}
		for j := range staleFilesPerFolder {
			rows = append(rows, inventoryfixture.Row{
				Path: folders[i] + "/gone" + strconv.Itoa(j) + ".mp3", RootPath: root, Size: 1,
			})
		}
		inventoryfixture.SeedInventory(t, repo, rows...)
	}
	return folders
}

func countInventoryFiles(t *testing.T, repo *sqlite.Repository) int {
	t.Helper()
	var n int
	if err := repo.DB().QueryRow("SELECT COUNT(*) FROM inventory_files").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSelectedMergeEmptiedFoldersAcrossBatchesNotifiesOnce(t *testing.T) {
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	root := filepath.ToSlash(t.TempDir())
	folders := seedEmptiedFolders(t, repo, root, 3)
	commits := 0
	repo.OnCatalogChanged = func() { commits++ }
	pipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo), filesystem.WalkRootEntriesParallel, filesystem.WalkFolderEntries, nil)
	if _, scanErr := pipeline.ScanFoldersCtx(t.Context(), folders, root, 2); scanErr != nil {
		t.Fatal(scanErr)
	}
	if got := countInventoryFiles(t, repo); got != 0 {
		t.Fatalf("stale files left: %d", got)
	}
	if commits != 1 {
		t.Fatalf("catalog notifications = %d, want 1 per merge", commits)
	}
}

func TestSelectedMergeFailureKeepsCommittedBatchesAndRescanConverges(t *testing.T) {
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	root := filepath.ToSlash(t.TempDir())
	folders := seedEmptiedFolders(t, repo, root, 3)
	catalog, merged := 0, 0
	repo.OnCatalogChanged = func() { catalog++ }
	pipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo), filesystem.WalkRootEntriesParallel, filesystem.WalkFolderEntries,
		func() { merged++ })
	// Each folder is its own batch: f1 fails, f0 is already committed.
	if _, execErr := repo.DB().Exec(`CREATE TRIGGER fail_second_batch BEFORE DELETE ON inventory_files
		WHEN OLD.dir_ref = (SELECT id FROM inventory_dirs WHERE path = '` + folders[1] + `')
		BEGIN SELECT RAISE(ABORT, 'injected'); END`); execErr != nil {
		t.Fatal(execErr)
	}
	if _, scanErr := pipeline.ScanFoldersCtx(t.Context(), folders, root, 2); scanErr == nil {
		t.Fatal("merge failure ignored")
	}
	if got := countInventoryFiles(t, repo); got != 2*staleFilesPerFolder {
		t.Fatalf("files after failed merge = %d, want the two uncommitted folders' %d", got, 2*staleFilesPerFolder)
	}
	var status, code string
	if rowErr := repo.DB().
		QueryRow("SELECT status, error_code FROM scan_sessions").Scan(&status, &code); rowErr != nil ||
		status != "failed" || code != "MERGE_FAILED" {
		t.Fatalf("session = %s/%s, %v", status, code, rowErr)
	}
	if catalog != 1 || merged != 1 {
		t.Fatalf("catalog=%d merged=%d, want 1 and 1 after a partial merge", catalog, merged)
	}
	if _, execErr := repo.DB().Exec("DROP TRIGGER fail_second_batch"); execErr != nil {
		t.Fatal(execErr)
	}
	if _, scanErr := pipeline.ScanFoldersCtx(t.Context(), folders, root, 2); scanErr != nil {
		t.Fatal(scanErr)
	}
	if got := countInventoryFiles(t, repo); got != 0 {
		t.Fatalf("rescan did not converge: %d files", got)
	}
}

func TestSelectedMergeCancelAfterBatchKeepsCommittedAndRescanConverges(t *testing.T) {
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	root := filepath.ToSlash(t.TempDir())
	folders := seedEmptiedFolders(t, repo, root, 3)
	catalog, merged := 0, 0
	repo.OnCatalogChanged = func() { catalog++ }
	pipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo), filesystem.WalkRootEntriesParallel, filesystem.WalkFolderEntries,
		func() { merged++ })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sqlite.SetAfterMergeBatch(repo, cancel)
	if _, scanErr := pipeline.ScanFoldersCtx(ctx, folders, root, 2); !errors.Is(scanErr, context.Canceled) {
		t.Fatalf("scan error = %v, want context canceled", scanErr)
	}
	if got := countInventoryFiles(t, repo); got != 2*staleFilesPerFolder {
		t.Fatalf("files after cancel = %d, want the two later folders' %d", got, 2*staleFilesPerFolder)
	}
	var status, code string
	if rowErr := repo.DB().
		QueryRow("SELECT status, error_code FROM scan_sessions").Scan(&status, &code); rowErr != nil ||
		status != "canceled" || code != "SCAN_CANCELLED" {
		t.Fatalf("session = %s/%s, %v", status, code, rowErr)
	}
	if catalog != 1 || merged != 1 {
		t.Fatalf("catalog=%d merged=%d, want 1 and 1 after a partial merge", catalog, merged)
	}
	sqlite.SetAfterMergeBatch(repo, nil)
	if _, scanErr := pipeline.ScanFoldersCtx(t.Context(), folders, root, 2); scanErr != nil {
		t.Fatal(scanErr)
	}
	if got := countInventoryFiles(t, repo); got != 0 {
		t.Fatalf("rescan did not converge: %d files", got)
	}
}
