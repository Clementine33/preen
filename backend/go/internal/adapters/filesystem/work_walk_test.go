package filesystem_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/inventory"
)

func TestWorkWalkDoesNotFollowLinksOrRepeatNestedWorkRoots(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "分类", "RJ000001", "RJ000002")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "track.mp3"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "分类"), filepath.Join(root, "RJ000003 link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	seen := map[string]inventory.DirEntry{}
	var mu sync.Mutex
	err := filesystem.WalkRootEntriesParallel(t.Context(), root, root, 1, func(e inventory.DirEntry) error {
		mu.Lock()
		defer mu.Unlock()
		if _, duplicate := seen[e.Path]; duplicate {
			t.Errorf("duplicate entry %s", e.Path)
		}
		seen[e.Path] = e
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	roots := 0
	for _, e := range seen {
		if e.IsWorkRoot {
			roots++
		}
	}
	if len(seen) != 5 || roots != 1 {
		t.Fatalf("entries=%d roots=%d: %+v", len(seen), roots, seen)
	}
	if seen[filepath.Join(root, "RJ000003 link")].IsDir {
		t.Fatal("directory link followed")
	}
}

func TestWorkWalkWorkerFailureStopsQueuedDiscovery(t *testing.T) {
	root := t.TempDir()
	for i := range 20 {
		folder := filepath.Join(root, fmt.Sprintf("RJ%06d", i+1))
		if err := os.Mkdir(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "track.mp3"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	failure := errors.New("worker failure")
	err := filesystem.WalkRootEntriesParallel(t.Context(), root, root, 1, func(e inventory.DirEntry) error {
		if e.Name == "track.mp3" {
			return failure
		}
		return nil
	})
	if !errors.Is(err, failure) {
		t.Fatalf("failure = %v", err)
	}
}
