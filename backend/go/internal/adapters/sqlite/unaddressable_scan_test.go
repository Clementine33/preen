package sqlite_test

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

func TestScanLeavesUnaddressableNamesOutOfInventory(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	root := filepath.ToSlash(t.TempDir())
	// A Shift_JIS directory name and a file name that is a lone 0xff byte.
	if err = os.MkdirAll(filepath.FromSlash(root+"/\x93\xfa"), 0o755); err != nil {
		t.Skipf("filesystem refuses non-UTF-8 names: %v", err)
	}
	if err = os.MkdirAll(filepath.FromSlash(root+"/ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"/\x93\xfa/a.mp3", "/ok/\xff.mp3", "/ok/b.mp3"} {
		if err = os.WriteFile(filepath.FromSlash(root+name), []byte("audio"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo),
		filesystem.WalkRootEntriesParallel,
		filesystem.WalkFolderEntries,
		nil,
	)
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	var last inventory.Progress
	report := inventory.WithProgress(func(p inventory.Progress) { last = p })
	if _, err = pipeline.ScanRootCtx(t.Context(), root, report); err != nil {
		t.Fatal(err)
	}
	// The directory is reported once; its child is never walked.
	if last.Unaddressable != 2 {
		t.Fatalf("unaddressable = %d, want 2", last.Unaddressable)
	}
	// Each refused name is logged once, escaped.
	for _, path := range []string{`path=\x93\xfa`, `path=ok/\xff.mp3`} {
		if !bytes.Contains(logs.Bytes(), []byte(path)) {
			t.Fatalf("log lacks %s:\n%s", path, logs.String())
		}
	}
	assertAllValid(t, repo, root)
	inventoryEntry(t, repo, root+"/ok/b.mp3")

	if _, err = pipeline.ScanFolderCtx(t.Context(), root+"/ok", root); err != nil {
		t.Fatal(err)
	}
	assertAllValid(t, repo, root)
}

func assertAllValid(t *testing.T, repo *sqlite.Repository, root string) {
	t.Helper()
	entries, err := repo.ListEntriesUnderPath(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !utf8.ValidString(entry.Path) {
			t.Fatalf("inventory holds unaddressable path %q", entry.Path)
		}
	}
}
