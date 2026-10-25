package sqlite_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

func TestScanSessionsLeaveNoStagingAcrossSuccessFailureAndCancellation(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	root := t.TempDir()
	if err = os.WriteFile(filepath.Join(root, "kept.mp3"), []byte("kept"), 0600); err != nil {
		t.Fatal(err)
	}
	walk := filesystem.WalkRootEntriesParallel
	pipeline := func() *inventory.Pipeline {
		return inventory.NewPipeline(sqlite.NewScanStaging(repo), walk, filesystem.WalkFolderEntries, nil)
	}
	if _, err = pipeline().ScanRootCtx(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	before, err := repo.ListEntriesUnderPath(filepath.ToSlash(root))
	if err != nil {
		t.Fatal(err)
	}
	// Produce more than a batch so failures leave committed TEMP rows before the walk ends.
	discarded := filepath.Join(root, "discarded")
	if err = os.Mkdir(discarded, 0700); err != nil {
		t.Fatal(err)
	}
	for i := range 1500 {
		if err = os.WriteFile(filepath.Join(discarded, fmt.Sprintf("file-%d", i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, cancelScan := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		walk = func(ctx context.Context, root, base string, workers int, emit func(inventory.DirEntry) error) error {
			if walkErr := filesystem.WalkRootEntriesParallel(ctx, root, base, workers, emit); walkErr != nil {
				return walkErr
			}
			if cancelScan {
				cancel()
				return ctx.Err()
			}
			return fmt.Errorf("walk failed after staging")
		}
		if _, err = pipeline().ScanRootCtx(ctx, root); err == nil {
			t.Fatal("failed scan succeeded")
		}
		cancel()
		after, readErr := repo.ListEntriesUnderPath(filepath.ToSlash(root))
		if readErr != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("failed scan changed inventory: %+v, %v", after, readErr)
		}
	}
	if err = os.RemoveAll(discarded); err != nil {
		t.Fatal(err)
	}
	walk = filesystem.WalkRootEntriesParallel
	if _, err = pipeline().ScanRootCtx(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	after, err := repo.ListEntriesUnderPath(filepath.ToSlash(root))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("later scan inherited staging: %+v, %v", after, err)
	}
}

func TestScanTempFilesInDataDirectory(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("spill location observed on Linux and Windows")
	}
	directory := os.Getenv("PREEN_TEST_TEMP_DIRECTORY")
	if directory == "" {
		//nolint:gosec // Re-exec this test binary to isolate SQLite process globals.
		command := exec.CommandContext(
			t.Context(),
			os.Args[0],
			"-test.run=^TestScanTempFilesInDataDirectory$",
			"-test.v",
		)
		command.Env = append(os.Environ(), "PREEN_TEST_TEMP_DIRECTORY="+t.TempDir())
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("temp directory subprocess: %v\n%s", err, output)
		}
		return
	}
	// SQLite's directory setting is process-global and must precede every connection.
	if err := sqlite.SetTempDirectory(directory); err != nil {
		t.Fatal(err)
	}
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(directory, "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store := sqlite.NewScanStaging(repo)
	entries := make([]inventory.StagingEntry, 1000)
	for batch := range 24 {
		for i := range entries {
			name := fmt.Sprintf("%06d-%s", batch*len(entries)+i, strings.Repeat("x", 220))
			entries[i] = inventory.StagingEntry{
				Path:       "/music/" + name,
				RootPath:   "/music",
				ParentPath: "/music",
				Name:       name,
			}
		}
		if err := store.WriteStagingBatch("spill", entries); err != nil {
			t.Fatal(err)
		}
	}
	spills := sqliteSpills(t, directory)
	if len(spills) == 0 {
		t.Fatal("no SQLite spill found in the data directory")
	}
	if err := store.CloseStagingSession("spill"); err != nil {
		t.Fatal(err)
	}
	if spills := sqliteSpills(t, directory); len(spills) != 0 {
		t.Fatalf("physical close retained spill files: %v", spills)
	}
}

func sqliteSpills(t *testing.T, directory string) []string {
	t.Helper()
	var spills []string
	if runtime.GOOS == "linux" {
		fds, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		for _, fd := range fds {
			target, err := os.Readlink("/proc/self/fd/" + fd.Name())
			if err != nil {
				continue
			}
			if strings.Contains(filepath.Base(target), "etilqs") {
				if filepath.Dir(target) != directory {
					t.Fatalf("SQLite spill outside data directory: %s", target)
				}
				spills = append(spills, target)
			}
		}
	} else {
		files, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.Contains(file.Name(), "etilqs") {
				spills = append(spills, file.Name())
			}
		}
	}
	return spills
}

func TestUnchangedRescanMainPagesDoNotGrowWithEntryCount(t *testing.T) {
	type pages struct{ total, free int }
	changes := make([]pages, 0, 2)
	for _, count := range []int{64, 4096} {
		repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer repo.Close()
		root := t.TempDir()
		for i := range count {
			if err = os.WriteFile(
				filepath.Join(root, fmt.Sprintf("%06d-%s.mp3", i, strings.Repeat("x", 180))),
				nil,
				0600,
			); err != nil {
				t.Fatal(err)
			}
		}
		pipeline := inventory.NewPipeline(
			sqlite.NewScanStaging(repo),
			filesystem.WalkRootEntriesParallel,
			filesystem.WalkFolderEntries,
			nil,
		)
		if _, err = pipeline.ScanRootCtx(t.Context(), root); err != nil {
			t.Fatal(err)
		}
		// Compact first so old main-database staging cannot hide in its existing freelist.
		if _, err = repo.DB().Exec("VACUUM"); err != nil {
			t.Fatal(err)
		}
		read := func() pages {
			var p pages
			if err = repo.DB().QueryRow("PRAGMA page_count").Scan(&p.total); err != nil {
				t.Fatal(err)
			}
			if err = repo.DB().QueryRow("PRAGMA freelist_count").Scan(&p.free); err != nil {
				t.Fatal(err)
			}
			return p
		}
		before := read()
		if _, err = pipeline.ScanRootCtx(t.Context(), root); err != nil {
			t.Fatal(err)
		}
		after := read()
		changes = append(changes, pages{after.total - before.total, after.free - before.free})
	}
	// Session metadata may split a handful of pages; staging must not scale with rows.
	if changes[1].total > changes[0].total+4 || changes[1].free > changes[0].free+4 {
		t.Fatalf("main page growth scaled with scan size: 64 rows=%+v,4096 rows=%+v", changes[0], changes[1])
	}
}
