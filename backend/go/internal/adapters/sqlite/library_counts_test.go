package sqlite_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"
)

func TestLibraryCountsKeepSubtreeBoundaries(t *testing.T) {
	for _, root := range []string{"/lib", "/", "C:/", "//server/share"} {
		t.Run(root, func(t *testing.T) {
			repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repo.Close() })
			prefix := strings.TrimSuffix(root, "/") + "/"
			inventoryfixture.SeedInventory(t, repo, inventoryfixture.Row{Path: root, RootPath: root, IsDir: true})
			for _, rel := range []string{"a", "a b", "a.", "aa", "Rock", "rock"} {
				inventoryfixture.SeedInventory(t, repo,
					inventoryfixture.Row{Path: prefix + rel, RootPath: root, IsDir: true, IsWorkRoot: true},
					inventoryfixture.Row{Path: prefix + rel + "/song.mp3", RootPath: root})
			}
			inventoryfixture.SeedInventory(
				t, repo, inventoryfixture.Row{Path: prefix + "a/disc/song.mp3", RootPath: root},
			)
			dirs, err := repo.ListLibraryDirs(root)
			if err != nil || len(dirs) != 6 {
				t.Fatalf("dirs = %+v, %v", dirs, err)
			}
			for _, d := range dirs {
				want := 1
				if d.RelPath == "a" {
					want = 2
				}
				if d.FileCount != want || d.AudioFileCount != want {
					t.Fatalf("counts for %q = %+v, want %d", d.RelPath, d, want)
				}
			}
			counts, err := repo.DirAudioCounts(root, []string{"a", "a b", "Rock", "rock", ""})
			if err != nil || counts["a"] != 2 || counts["a b"] != 1 ||
				counts["Rock"] != 1 || counts["rock"] != 1 || counts[""] != 7 {
				t.Fatalf("counts = %+v, %v", counts, err)
			}
		})
	}
}
