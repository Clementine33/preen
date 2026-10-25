package sqlite_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/librarypath"
)

//nolint:funlen // Covers full scan, no-op rescan and scoped refresh as one lifecycle.
func TestNestedWorkInventoryAndNoOpRescan(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"分类/A/RJ000001 first/RJ000002 inside/track.flac",
		"分类/B/RJ000001 second/track.flac",
		"RJ000002 RJ000003/RJ000004 work/track.flac",
		"Delete/RJ000005 recovered/track.flac",
		"分类/A/Delete/RJ000006 recovered/track.flac",
	} {
		file := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	lib, err := repo.CreateLibrary("test", root)
	if err != nil {
		t.Fatal(err)
	}
	// An older scan included recovered media; a full rescan must remove those rows.
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: librarypath.JoinRel(root, "Delete"), RootPath: root, IsDir: true},
		inventoryfixture.Row{Path: librarypath.JoinRel(root, "Delete/RJ000005 recovered/track.flac"), RootPath: root},
	)
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{
			Path:       librarypath.JoinRel(root, "分类/A/Delete/RJ000006 recovered"),
			RootPath:   root,
			IsDir:      true,
			IsWorkRoot: true,
		},
	)
	// Existing classification ancestors carry their observed facts, so the first scan is clean.
	for _, rel := range []string{"分类", "分类/A"} {
		path := librarypath.JoinRel(root, rel)
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		inventoryfixture.SeedInventory(
			t,
			repo,
			inventoryfixture.Row{
				Path:     path,
				RootPath: root,
				IsDir:    true,
				Size:     info.Size(),
				Mtime:    info.ModTime().Unix(),
			},
		)
	}
	pipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo),
		filesystem.WalkRootEntriesParallel,
		filesystem.WalkFolderEntries,
		nil,
	)
	scan := func() string {
		t.Helper()
		id, scanErr := pipeline.ScanRootCtx(t.Context(), root, inventory.WithLibrary(lib.ID))
		if scanErr != nil {
			t.Fatal(scanErr)
		}
		return id
	}
	scan()
	recovered, err := repo.ListEntriesUnderPath(librarypath.JoinRel(root, "Delete"))
	if err != nil || len(recovered) != 0 {
		t.Fatalf("recovery inventory = %v, err = %v", recovered, err)
	}
	recovered, err = repo.ListEntriesUnderPath(librarypath.JoinRel(root, "分类/A/Delete"))
	if err != nil || len(recovered) != 0 {
		t.Fatalf("nested recovery inventory = %v, err = %v", recovered, err)
	}
	roots, err := repo.ListLibraryWorkRoots(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"RJ000002 RJ000003/RJ000004 work", "分类/A/RJ000001 first", "分类/B/RJ000001 second"}
	if !reflect.DeepEqual(roots, want) {
		t.Fatalf("roots = %v", roots)
	}
	var copies, works int
	if err = repo.DB().
		QueryRow("SELECT count(*), count(DISTINCT work_id) FROM work_copies").
		Scan(&copies, &works); err != nil {
		t.Fatal(err)
	}
	if copies != 3 || works != 2 {
		t.Fatalf("copies/works = %d/%d", copies, works)
	}
	var copyID string
	if err = repo.DB().QueryRow("SELECT id FROM work_copies WHERE rel_path = ?", want[1]).Scan(&copyID); err != nil {
		t.Fatal(err)
	}
	writeState := func() string {
		t.Helper()
		var state string
		if stateErr := repo.DB().
			QueryRow("SELECT group_concat(path || ':' || scan_id || ':' || updated_at, char(10) ORDER BY path) FROM entries").
			Scan(&state); stateErr != nil {
			t.Fatal(stateErr)
		}
		return state
	}
	before := writeState()
	second := scan()
	if after := writeState(); before != after {
		t.Fatalf("no-op scan rewrote inventory: before=%q after=%q", before, after)
	}
	var staging int
	if err = repo.DB().QueryRow("SELECT count(*) FROM entries_staging").Scan(&staging); err != nil {
		t.Fatal(err)
	}
	if staging != 0 {
		t.Fatalf("staging = %d", staging)
	}
	var status, afterID string
	if err = repo.DB().
		QueryRow("SELECT status FROM scan_sessions WHERE session_id = ?", second).
		Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatal(status)
	}
	if err = repo.DB().QueryRow("SELECT id FROM work_copies WHERE rel_path = ?", want[1]).Scan(&afterID); err != nil {
		t.Fatal(err)
	}
	if copyID != afterID {
		t.Fatal("copy identity changed")
	}
	if _, err = pipeline.ScanFolderCtx(
		context.Background(),
		filepath.Join(root, filepath.FromSlash(want[1])),
		root,
	); err != nil {
		t.Fatal(err)
	}
	roots, err = repo.ListLibraryWorkRoots(root)
	if err != nil || !reflect.DeepEqual(roots, want) {
		t.Fatalf("refresh roots = %v, %v", roots, err)
	}
}

func TestLibraryListingsExcludeLegacyNestedRecovery(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/music/分类/Delete/RJ000006", RootPath: "/music", IsDir: true, IsWorkRoot: true},
	)
	listed, err := repo.ListLibraryDirs("/music")
	if err != nil || len(listed) != 0 {
		t.Fatalf("legacy recovery listed as work = %v, err = %v", listed, err)
	}
	roots, err := repo.ListLibraryWorkRoots("/music")
	if err != nil || len(roots) != 0 {
		t.Fatalf("legacy recovery accepted as member = %v, err = %v", roots, err)
	}
}

func TestMergeKeepsFactsUntilContentChangesAndCleansDirty(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	store := sqlite.NewScanStaging(repo)
	entry := inventory.StagingEntry{
		Path:       "/music/item",
		RootPath:   "/music",
		ParentPath: "/music",
		Name:       "item",
		Size:       10,
		Mtime:      20,
	}
	merge := func(session string) {
		t.Helper()
		entry.SessionID = session
		if err := store.WriteStagingEntries(session, []inventory.StagingEntry{entry}); err != nil {
			t.Fatal(err)
		}
		outcome, err := repo.MergeStaging(session, "", "/music")
		if err != nil || outcome.Merged != 1 {
			t.Fatalf("merge = %+v, %v", outcome, err)
		}
	}
	merge("first")
	if _, err := repo.DB().
		Exec("UPDATE entries SET bitrate=320000,width=100,height=200,duration_sec=3,is_error=1,error_reason='saved',updated_at='unchanged'"); err != nil {
		t.Fatal(err)
	}
	merge("same")
	var session, updated string
	if err := repo.DB().QueryRow("SELECT scan_id,updated_at FROM entries").Scan(&session, &updated); err != nil {
		t.Fatal(err)
	}
	if session != "first" || updated != "unchanged" {
		t.Fatal("unchanged row was updated")
	}
	entry.Name = "changed observation"
	merge("name")
	var rev, dirty, isError int
	var bitrate sql.NullInt64
	if err := repo.DB().
		QueryRow("SELECT content_rev,dirty_flag,is_error,bitrate FROM entries").
		Scan(&rev, &dirty, &isError, &bitrate); err != nil {
		t.Fatal(err)
	}
	if rev != 1 || !bitrate.Valid || isError != 1 {
		t.Fatal("observation update invalidated facts or errors")
	}
	entry.IsDir = true
	merge("type")
	if err := repo.DB().
		QueryRow("SELECT content_rev,dirty_flag,is_error,bitrate FROM entries").
		Scan(&rev, &dirty, &isError, &bitrate); err != nil {
		t.Fatal(err)
	}
	if rev != 2 || dirty != 1 || bitrate.Valid || isError != 1 {
		t.Fatal("type change lifecycle is incorrect")
	}
	merge("clean")
	if err := repo.DB().
		QueryRow("SELECT content_rev,dirty_flag,scan_id FROM entries").
		Scan(&rev, &dirty, &session); err != nil {
		t.Fatal(err)
	}
	if rev != 2 || dirty != 0 || session != "clean" {
		t.Fatal("unchanged dirty row was not cleaned")
	}
}

func TestWorkRootsKeepWindowsRootForms(t *testing.T) {
	for _, root := range []string{"C:/", "C:/Library", "//SERVER/Share"} {
		t.Run(root, func(t *testing.T) {
			repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repo.Close() })
			lib, err := repo.CreateLibrary("windows", root)
			if err != nil {
				t.Fatal(err)
			}
			rel := "分类/RJ000001"
			folder := librarypath.JoinRel(root, rel)
			entry := inventory.StagingEntry{
				SessionID:  "scan",
				RootPath:   root,
				Path:       folder,
				ParentPath: librarypath.JoinRel(root, "分类"),
				Name:       "RJ000001",
				IsDir:      true,
				IsWorkRoot: true,
			}
			if err = sqlite.NewScanStaging(repo).
				WriteStagingEntries("scan", []inventory.StagingEntry{entry}); err != nil {
				t.Fatal(err)
			}
			if _, err = repo.MergeStaging("scan", lib.ID, root); err != nil {
				t.Fatal(err)
			}
			roots, err := repo.ListLibraryWorkRoots(root)
			if err != nil || !reflect.DeepEqual(roots, []string{rel}) {
				t.Fatalf("roots=%v err=%v", roots, err)
			}
			targets, err := repo.CoverTargets(t.Context(), root, []string{folder})
			if err != nil || targets[folder].RelPath != rel {
				t.Fatalf("targets=%v err=%v", targets, err)
			}
			names, err := repo.RenameWorks(t.Context(), root, []string{folder})
			if err != nil || names[folder].Code != "RJ00000001" {
				t.Fatalf("names=%v err=%v", names, err)
			}
		})
	}
}

func TestScopedRootRefreshKeepsItsOwnRowAndOtherInventory(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	inventoryfixture.SeedInventory(t, repo, inventoryfixture.Row{Path: "/", RootPath: "/", IsDir: true},
		inventoryfixture.Row{Path: "/stale", RootPath: "/"},
		inventoryfixture.Row{Path: "/foreign", RootPath: "/another"})
	pipeline := inventory.NewPipeline(sqlite.NewScanStaging(repo), filesystem.WalkRootEntriesParallel,
		func(context.Context, string, string, func(inventory.DirEntry) error) error { return nil }, nil)
	if _, err = pipeline.ScanFolderCtx(t.Context(), "/", "/"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/foreign"} {
		found, err := repo.RootExistsInInventory(path)
		if err != nil || !found {
			t.Fatalf("preserved %s: found=%v err=%v", path, found, err)
		}
	}
	var rows int
	{
		found, readErr := repo.RootExistsInInventory("/stale")
		if readErr != nil {
			t.Fatal(readErr)
		}
		rows = 0
		if found {
			rows = 1
		}
	}
	if rows != 0 {
		t.Fatalf("stale=%d nil=%v", rows, nil)
	}
}
