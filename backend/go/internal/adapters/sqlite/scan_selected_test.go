package sqlite_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/inventory"
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
		if _, err := repo.DB().
			Exec(`INSERT INTO entries(path, root_path, parent_path, name, is_dir, is_work_root, size, mtime, format, bitrate, duration_sec, content_rev, dirty_flag)
			VALUES(?, ?, ?, ?, ?, ?, 5, ?, 'mpeg', 320000, 12, 7, 0)`, path, root, filepath.ToSlash(filepath.Dir(path)), filepath.Base(path), path == scope, path == scope, info.ModTime().Unix()); err != nil {
			t.Fatal(err)
		}
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
		var count int
		if err := repo.DB().
			QueryRow("SELECT COUNT(*) FROM entries WHERE path = ?", path).
			Scan(&count); err != nil ||
			count != 1 {
			t.Fatalf("preserved %s = %d, %v", path, count, err)
		}
	}
	var copyID string
	if bindErr := repo.DB().
		QueryRow("SELECT id FROM work_copies WHERE library_id = 'selected-library' AND rel_path = 'a'").
		Scan(&copyID); bindErr != nil ||
		copyID != "selected-copy" {
		t.Fatalf("binding changed: %s, %v", copyID, bindErr)
	}

	var count, revision, bitrate int
	var duration float64
	if err := repo.DB().
		QueryRow("SELECT COUNT(*) FROM entries WHERE path = ?", scope+"/deleted.mp3").
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatalf("stale row = %d, %v", count, err)
	}
	if err := repo.DB().
		QueryRow("SELECT content_rev, bitrate, duration_sec FROM entries WHERE path = ?", file).
		Scan(&revision, &bitrate, &duration); err != nil {
		t.Fatal(err)
	}
	if revision != 7 || bitrate != 320000 || duration != 12 {
		t.Fatalf("unchanged media facts changed: %d %d %f", revision, bitrate, duration)
	}
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
	if _, err := repo.DB().
		Exec(`CREATE TRIGGER reject_selected_cleanup BEFORE DELETE ON entries_staging BEGIN SELECT RAISE(ABORT, 'cleanup failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeSelectedStaging(t.Context(), "selected", "/library", []string{"/library/a"}); err == nil {
		t.Fatal("merge failure ignored")
	}
	var count int
	if err := repo.DB().QueryRow("SELECT COUNT(*) FROM entries").Scan(&count); err != nil || count != 0 {
		t.Fatalf("merge did not roll back: %d, %v", count, err)
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
		Exec("INSERT INTO works(id, created_at, updated_at) VALUES('RJ000001', 'created', 'updated')"); err != nil {
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
