package sqlite_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

func TestScopedRefreshClearsUnobservedRootMtime(t *testing.T) {
	for _, selected := range []bool{false, true} {
		name := "member"
		if selected {
			name = "selected"
		}
		t.Run(name, func(t *testing.T) {
			repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			root := filepath.ToSlash(t.TempDir())
			scope := root + "/RJ000001"
			if err = os.MkdirAll(filepath.FromSlash(scope+"/child"), 0o755); err != nil {
				t.Fatal(err)
			}
			setDirectoryMtime(t, scope, 100)
			setDirectoryMtime(t, scope+"/child", 200)
			pipeline := inventory.NewPipeline(
				sqlite.NewScanStaging(repo),
				filesystem.WalkRootEntriesParallel,
				filesystem.WalkFolderEntries,
				nil,
			)
			if _, err = pipeline.ScanRootCtx(t.Context(), root); err != nil {
				t.Fatal(err)
			}
			before := inventoryEntry(t, repo, scope)
			beforeDirs, err := repo.ListLibraryDirs(root)
			if err != nil || len(beforeDirs) != 1 {
				t.Fatalf("work root before refresh: %+v, %v", beforeDirs, err)
			}
			if before.Mtime != 100 || inventoryEntry(t, repo, scope+"/child").Mtime != 200 {
				t.Fatalf("full scan did not preserve observed directory mtimes: %+v", before)
			}
			if err = os.WriteFile(filepath.FromSlash(scope+"/new.mp3"), []byte("audio"), 0o600); err != nil {
				t.Fatal(err)
			}
			setDirectoryMtime(t, scope, 300)
			setDirectoryMtime(t, scope+"/child", 400)
			if selected {
				_, err = pipeline.ScanFoldersCtx(t.Context(), []string{scope}, root, 1)
			} else {
				_, err = pipeline.ScanFolderCtx(t.Context(), scope, root)
			}
			if err != nil {
				t.Fatal(err)
			}
			after := inventoryEntry(t, repo, scope)
			if after.Mtime != 0 || inventoryEntry(t, repo, scope+"/child").Mtime != 400 {
				t.Fatalf("refresh kept unobserved mtime or lost observed child mtime: %+v", after)
			}
			afterDirs, err := repo.ListLibraryDirs(root)
			if err != nil || len(afterDirs) != 1 || !afterDirs[0].AddedAt.Equal(beforeDirs[0].AddedAt) {
				t.Fatalf(
					"refresh changed work root or collection time: before=%+v after=%+v, %v",
					beforeDirs,
					afterDirs,
					err,
				)
			}
			inventoryEntry(t, repo, scope+"/new.mp3")
		})
	}
}

func setDirectoryMtime(t *testing.T, path string, seconds int64) {
	t.Helper()
	stamp := time.Unix(seconds, 0)
	if err := os.Chtimes(filepath.FromSlash(path), stamp, stamp); err != nil {
		t.Fatal(err)
	}
}

func TestSyntheticDirectoryMtimesStayUnknown(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	root := filepath.ToSlash(t.TempDir())
	output := root + "/created/nested/track.mp3"
	if err := repo.SyncObservedInventory(
		root,
		nil,
		[]inventory.InventoryFile{{Path: output, Size: 5, Mtime: 600}},
		nil,
	); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root + "/created", root + "/created/nested"} {
		if got := inventoryEntry(t, repo, path); got.Mtime != 0 {
			t.Fatalf("execution ancestor has a trusted mtime: %+v", got)
		}
	}
	store := sqlite.NewScanStaging(repo)
	if err := store.CreateScanSession(&inventory.ScanSession{
		SessionID: "file-only", RootPath: root, Kind: "full", Status: "running", StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.CloseStagingSession("file-only") }()
	if err := store.WriteStagingBatch("file-only", []inventory.StagingEntry{{
		Path: root + "/merged/track.mp3", RootPath: root, ParentPath: root + "/merged", Name: "track.mp3", Mtime: 700,
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeStaging("file-only", "", root); err != nil {
		t.Fatal(err)
	}
	if got := inventoryEntry(t, repo, root+"/merged"); got.Mtime != 0 {
		t.Fatalf("file-only merge parent has a trusted mtime: %+v", got)
	}
}

func TestDirectoryRelocationsPreserveObservedMtimes(t *testing.T) {
	for _, direct := range []bool{false, true} {
		name := "rename execution"
		if direct {
			name = "direct move"
		}
		t.Run(name, func(t *testing.T) {
			repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			root := filepath.ToSlash(t.TempDir())
			source, target := root+"/old", root+"/new"
			relocate := repo.RelocateDirectory
			if direct {
				target = root + "/destination/old"
				relocate = repo.MoveInventoryPath
			}
			for _, dir := range []string{source + "/nested", root + "/destination"} {
				if err = os.MkdirAll(filepath.FromSlash(dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err = os.WriteFile(filepath.FromSlash(source+"/nested/track.mp3"), []byte("audio"), 0o600); err != nil {
				t.Fatal(err)
			}
			setDirectoryMtime(t, source, 100)
			setDirectoryMtime(t, source+"/nested", 200)
			pipeline := inventory.NewPipeline(
				sqlite.NewScanStaging(repo),
				filesystem.WalkRootEntriesParallel,
				filesystem.WalkFolderEntries,
				nil,
			)
			if _, err = pipeline.ScanRootCtx(t.Context(), root); err != nil {
				t.Fatal(err)
			}
			if err = os.Rename(filepath.FromSlash(source), filepath.FromSlash(target)); err != nil {
				t.Fatal(err)
			}
			if err = relocate(t.Context(), root, source, target); err != nil {
				t.Fatal(err)
			}
			if got := inventoryEntry(t, repo, target); got.Mtime != 100 {
				t.Fatalf("relocation changed source mtime: %+v", got)
			}
			if got := inventoryEntry(t, repo, target+"/nested"); got.Mtime != 200 {
				t.Fatalf("relocation changed descendant mtime: %+v", got)
			}
			if _, err = pipeline.ScanRootCtx(t.Context(), root); err != nil {
				t.Fatal(err)
			}
			inventoryEntry(t, repo, target+"/nested/track.mp3")
		})
	}
}
