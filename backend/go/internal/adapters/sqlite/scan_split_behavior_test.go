package sqlite_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"
)

func TestScanReplacesPathTypesAndKeepsTheLibraryRootHidden(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	root := filepath.ToSlash(t.TempDir())
	target := root + "/RJ000001"
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: target, RootPath: root, ContentRev: 8, Bitrate: new(int32(320000))},
	)
	if err = os.Mkdir(filepath.FromSlash(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.FromSlash(target+"/track.mp3"), []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	pipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo),
		filesystem.WalkRootEntriesParallel,
		filesystem.WalkFolderEntries,
		nil,
	)
	var progress inventory.Progress
	if _, err = pipeline.ScanRootCtx(
		t.Context(),
		root,
		inventory.WithProgress(func(p inventory.Progress) { progress = p }),
	); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListEntriesUnderPath(root)
	if err != nil || len(rows) != 2 || rows[0].Path != target || !rows[0].IsDir || progress.DirsScanned != 1 ||
		progress.FilesScanned != 1 {
		t.Fatalf("directory replacement=%+v progress=%+v, %v", rows, progress, err)
	}
	if err = os.RemoveAll(filepath.FromSlash(target)); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.FromSlash(target), []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = pipeline.ScanRootCtx(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	rows, err = repo.ListEntriesUnderPath(root)
	if err != nil || len(rows) != 1 || rows[0].Path != target || rows[0].IsDir || rows[0].ContentRev != 1 ||
		rows[0].Bitrate != nil {
		t.Fatalf("file replacement=%+v, %v", rows, err)
	}
	dirs, err := repo.ListLibraryDirs(root)
	if err != nil || len(dirs) != 0 {
		t.Fatalf("root directory leaked=%+v, %v", dirs, err)
	}
}
