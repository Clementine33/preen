package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
)

// TestObservedAudioEntriesSpansTheWholeSubtree pins the path boundaries a
// planning root is collected with: the root directory itself ("/", kept
// defensively even though no member folder can be it), a root spelled with a
// trailing slash, and a root whose audio sits several directories down. The
// range predicate is a binary one over the path index, so its edges are the
// contract — a sibling whose name merely starts with the root's name is a
// different subtree.
func TestObservedAudioEntriesSpansTheWholeSubtree(t *testing.T) {
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	now := time.Now().Format(time.RFC3339Nano)
	for _, path := range []string{
		"/music/a/01.mp3",
		"/music/a/deep/02.mp3",
		"/music/ab/03.mp3",
		"/other/04.mp3",
	} {
		inventoryfixture.SeedInventory(
			t,
			repo,
			inventoryfixture.Row{
				Path:      path,
				RootPath:  filepath.Dir(path),
				Format:    "mp3",
				CreatedAt: now,
				UpdatedAt: now,
			},
		)
	}
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/music/a/deep", RootPath: "/music/a", IsDir: true, CreatedAt: now, UpdatedAt: now},
	)

	for _, tc := range []struct {
		root string
		want []string
	}{
		{"/", []string{"/music/a/01.mp3", "/music/a/deep/02.mp3", "/music/ab/03.mp3", "/other/04.mp3"}},
		{"/music/", []string{"/music/a/01.mp3", "/music/a/deep/02.mp3", "/music/ab/03.mp3"}},
		{"/music/a", []string{"/music/a/01.mp3", "/music/a/deep/02.mp3"}},
		{"/music/a/", []string{"/music/a/01.mp3", "/music/a/deep/02.mp3"}},
		{"/music/ab", []string{"/music/ab/03.mp3"}},
	} {
		entries, err := repo.ObservedAudioEntries(tc.root)
		if err != nil {
			t.Fatalf("collect %s: %v", tc.root, err)
		}
		got := make([]string, 0, len(entries))
		for _, e := range entries {
			got = append(got, e.Path)
		}
		if len(got) != len(tc.want) {
			t.Errorf("root %q collected %v, want %v", tc.root, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("root %q collected %v, want %v", tc.root, got, tc.want)
				break
			}
		}
	}
}
