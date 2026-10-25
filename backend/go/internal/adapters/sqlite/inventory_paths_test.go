package sqlite_test

import (
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"
)

func TestExecutionInventoryPreservesStoredPathParts(t *testing.T) {
	for _, tc := range []struct{ path, parent, name string }{
		{"/track.mp3", "/", "track.mp3"},
		{"C:/track.mp3", "C:/", "track.mp3"},
		{"//server/share/track.mp3", "//server/share", "track.mp3"},
		{"/music/作品/track.mp3", "/music/作品", "track.mp3"},
		{"/music/album:/track.mp3", "/music/album:", "track.mp3"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "cache.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			for _, size := range []int64{1, 2} {
				if err := repo.SyncObservedInventory(
					tc.parent, nil, []inventory.InventoryFile{{Path: tc.path, Size: size}}, nil,
				); err != nil {
					t.Fatal(err)
				}
				got, found, err := repo.EntryByPath(tc.path)
				if err != nil || !found || got.Path != tc.path || got.ParentPath != tc.parent || got.Name != tc.name {
					t.Fatalf(
						"stored path = %+v, found=%v, err=%v; want parent %q, name %q",
						got, found, err, tc.parent, tc.name,
					)
				}
			}
		})
	}
}

func TestInventoryRangesKeepSiblingPathsIsolated(t *testing.T) {
	for _, tc := range []struct{ root, prefix string }{
		{"/", "/"}, {"C:/", "C:/"}, {"//server/share", "//server/share/"}, {"/music", "/music/"},
	} {
		t.Run(tc.root, func(t *testing.T) {
			repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "cache.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			exerciseInventoryPathRanges(t, repo, tc.root, tc.prefix)
		})
	}
}

func exerciseInventoryPathRanges(t *testing.T, repo *sqlite.Repository, root, prefix string) {
	t.Helper()
	scope := prefix + "a%_日"
	for _, name := range []string{"a%_日", "a%_日 b", "A%_日"} {
		inventoryfixture.SeedInventory(t, repo,
			inventoryfixture.Row{Path: prefix + name, RootPath: root, IsDir: true, IsWorkRoot: true},
			inventoryfixture.Row{Path: prefix + name + "/keep.mp3", RootPath: root},
		)
	}
	inventoryfixture.SeedInventory(t, repo,
		inventoryfixture.Row{Path: scope + "/deep/gone.mp3", RootPath: root},
		inventoryfixture.Row{Path: prefix + "direct.mp3", RootPath: root},
	)
	dirs, err := repo.ListLibraryDirs(root)
	if err != nil || len(dirs) != 3 {
		t.Fatalf("overview = %+v, err=%v; want 3 directories", dirs, err)
	}
	for _, dir := range dirs {
		want := 1
		if dir.Path == scope {
			want = 2
		}
		if dir.AudioFileCount != want || dir.FileCount != want {
			t.Fatalf("overview directory = %+v; want %d files and audio", dir, want)
		}
	}
	audio, err := repo.ObservedAudioEntries(scope + "/")
	if err != nil || len(audio) != 2 {
		t.Fatalf("observed audio = %+v, err=%v; want 2", audio, err)
	}
	entries, err := repo.ListEntriesUnderPath(scope)
	var paths []string
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	want := []string{scope, scope + "/deep", scope + "/deep/gone.mp3", scope + "/keep.mp3"}
	if err != nil || !slices.Equal(paths, want) {
		t.Fatalf("subtree = %v, err=%v; want %v", paths, err, want)
	}
	mergeSelectedPathRange(t, repo, root, scope)
	for _, path := range []string{
		scope, scope + "/keep.mp3", prefix + "a%_日 b/keep.mp3", prefix + "A%_日/keep.mp3", prefix + "direct.mp3",
	} {
		found, readErr := repo.RootExistsInInventory(path)
		if readErr != nil || !found {
			t.Fatalf("preserved %q: found=%v err=%v", path, found, readErr)
		}
	}
	found, err := repo.RootExistsInInventory(scope + "/deep/gone.mp3")
	if err != nil || found {
		t.Fatalf("stale descendant: found=%v err=%v", found, err)
	}
	target := prefix + "renamed"
	if moveErr := repo.RelocateDirectory(t.Context(), root, scope, target); moveErr != nil {
		t.Fatal(moveErr)
	}
	moved, found, err := repo.EntryByPath(target + "/keep.mp3")
	if err != nil || !found || moved.ParentPath != target || moved.Name != "keep.mp3" {
		t.Fatalf("moved descendant = %+v, found=%v err=%v", moved, found, err)
	}
	for _, path := range []string{prefix + "a%_日 b/keep.mp3", prefix + "A%_日/keep.mp3"} {
		if _, found, err := repo.EntryByPath(path); err != nil || !found {
			t.Fatalf("relocation changed sibling %q: found=%v err=%v", path, found, err)
		}
	}
}

func TestInventoryRootScanCleanupKeepsOtherLibraries(t *testing.T) {
	for _, tc := range []struct{ root, direct, stale string }{
		{"/", "/keep.mp3", "/deep/gone.mp3"},
		{"C:/", "C:/keep.mp3", "C:/deep/gone.mp3"},
		{"//server/share", "//server/share/keep.mp3", "//server/share/deep/gone.mp3"},
		{"/music", "/music/keep.mp3", "/music/deep/gone.mp3"},
	} {
		for _, scoped := range []bool{false, true} {
			t.Run(tc.root+"/scoped="+strconv.FormatBool(scoped), func(t *testing.T) {
				repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "cache.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer repo.Close()
				inventoryfixture.SeedInventory(t, repo,
					inventoryfixture.Row{Path: tc.direct, RootPath: tc.root},
					inventoryfixture.Row{Path: tc.stale, RootPath: tc.root},
					inventoryfixture.Row{Path: "D:/other/keep.mp3", RootPath: "D:/other"},
				)
				store := sqlite.NewScanStaging(repo)
				session := &inventory.ScanSession{
					SessionID: "scan", RootPath: tc.root, Kind: "root", Status: "running", StartedAt: time.Now(),
				}
				if scoped {
					session.Kind, session.ScopePath = "folder", &tc.root
				}
				if err := store.CreateScanSession(session); err != nil {
					t.Fatal(err)
				}
				if err := store.WriteStagingEntries("scan", []inventory.StagingEntry{{
					SessionID: "scan", RootPath: tc.root, Path: tc.direct, ParentPath: tc.root, Name: "keep.mp3",
				}}); err != nil {
					t.Fatal(err)
				}
				if _, err := repo.MergeStaging("scan", "", tc.root); err != nil {
					t.Fatal(err)
				}
				for _, path := range []string{tc.direct, "D:/other/keep.mp3"} {
					if _, found, err := repo.EntryByPath(path); err != nil || !found {
						t.Fatalf("cleanup removed %q: found=%v err=%v", path, found, err)
					}
				}
				if _, found, err := repo.EntryByPath(tc.stale); err != nil || found {
					t.Fatalf("cleanup left stale descendant: found=%v err=%v", found, err)
				}
			})
		}
	}
}

func mergeSelectedPathRange(t *testing.T, repo *sqlite.Repository, root, scope string) {
	t.Helper()
	store := sqlite.NewScanStaging(repo)
	if err := store.CreateScanSession(&inventory.ScanSession{
		SessionID: "selected", RootPath: root, Kind: "selected", Status: "running", StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteStagingBatch("selected", []inventory.StagingEntry{{
		SessionID: "selected", RootPath: root, Path: scope + "/keep.mp3", ParentPath: scope, Name: "keep.mp3",
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeSelectedStaging(t.Context(), "selected", root, []string{scope}); err != nil {
		t.Fatal(err)
	}
}

func TestInventoryRootCountsIncludeDirectAndNestedFiles(t *testing.T) {
	for _, tc := range []struct{ root, direct, nested string }{
		{"/", "/direct.mp3", "/album/track.mp3"},
		{"C:/", "C:/direct.mp3", "C:/album/track.mp3"},
		{"//server/share", "//server/share/direct.mp3", "//server/share/album/track.mp3"},
		{"/music", "/music/direct.mp3", "/music/album/track.mp3"},
	} {
		t.Run(tc.root, func(t *testing.T) {
			repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "cache.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			inventoryfixture.SeedInventory(t, repo,
				inventoryfixture.Row{Path: tc.root, RootPath: tc.root, IsDir: true},
				inventoryfixture.Row{Path: tc.direct, RootPath: tc.root},
				inventoryfixture.Row{Path: tc.nested, RootPath: tc.root},
			)
			counts, err := repo.DirAudioCounts(tc.root, []string{""})
			if err != nil || counts[""] != 2 {
				t.Fatalf("root audio counts = %v, err=%v; want 2", counts, err)
			}
		})
	}
}
