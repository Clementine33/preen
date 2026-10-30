package sqlite //nolint:testpackage // Verifies split-table storage invariants while public readers migrate separately.

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"
)

func TestSplitMergePreservesDirectoryIdentityAndUnchangedFacts(t *testing.T) {
	repo := newTestRepository(t)
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/library/work", RootPath: "/library", IsDir: true, Mtime: 20},
		inventoryfixture.Row{
			Path:        "/library/work/keep.MP3",
			RootPath:    "/library",
			Size:        10,
			Mtime:       20,
			ContentRev:  7,
			Bitrate:     new(int32(320000)),
			Width:       new(30),
			Height:      new(40),
			DurationSec: new(float64(12)),
		},
	)
	var before int64
	if err := repo.DB().QueryRow("SELECT id FROM inventory_dirs WHERE path='/library/work'").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().
		Exec(`CREATE TRIGGER reject_file_update BEFORE UPDATE ON inventory_files BEGIN SELECT RAISE(ABORT, 'unchanged file rewritten'); END`); err != nil {
		t.Fatal(err)
	}
	stageSplitRows(
		t,
		repo,
		"same",
		inventory.StagingEntry{Path: "/library/work", IsDir: true, Mtime: 20},
		inventory.StagingEntry{Path: "/library/work/keep.MP3", Size: 10, Mtime: 20},
	)
	outcome, err := repo.MergeStaging("same", "", "/library")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Merged != 2 {
		t.Fatalf("merged=%d, want 2", outcome.Merged)
	}
	var id int64
	var rev, rate, width, height, audio int
	var duration float64
	err = repo.DB().
		QueryRow(`SELECT d.id,f.content_rev,f.bitrate,f.width,f.height,f.duration_sec,f.is_audio FROM inventory_dirs d JOIN inventory_files f ON f.dir_ref=d.id WHERE d.path='/library/work' AND f.name='keep.MP3'`).
		Scan(&id, &rev, &rate, &width, &height, &duration, &audio)
	if err != nil || id != before || rev != 7 || rate != 320000 || width != 30 || height != 40 || duration != 12 ||
		audio != 1 {
		t.Fatalf("stored facts=%d %d %d %d %d %g %d, err=%v", id, rev, rate, width, height, duration, audio, err)
	}
	var rootWork int
	if err := repo.DB().
		QueryRow("SELECT is_work_root FROM inventory_dirs WHERE path='/library'").
		Scan(&rootWork); err != nil ||
		rootWork != 0 {
		t.Fatalf("root marker=%d, %v", rootWork, err)
	}
}

func stageSplitRows(t *testing.T, repo *Repository, id string, rows ...inventory.StagingEntry) {
	t.Helper()
	for i := range rows {
		rows[i].SessionID = id
		rows[i].RootPath = "/library"
		rows[i].ParentPath, rows[i].Name = splitInventoryPath(rows[i].Path)
	}
	if err := NewScanStaging(repo).WriteStagingEntries(id, rows); err != nil {
		t.Fatal(err)
	}
}

func splitFileRevision(t *testing.T, repo *Repository, dir, name string) (int, sql.NullInt64) {
	t.Helper()
	var rev int
	var bitrate sql.NullInt64
	if err := repo.DB().
		QueryRow(`SELECT f.content_rev,f.bitrate FROM inventory_files f JOIN inventory_dirs d ON d.id=f.dir_ref WHERE d.path=? AND f.name=?`, dir, name).
		Scan(&rev, &bitrate); err != nil {
		t.Fatal(err)
	}
	return rev, bitrate
}

func TestSplitScanReplacesAFileScopeWithADirectory(t *testing.T) {
	for _, kind := range []string{"selected", "folder"} {
		t.Run(kind, func(t *testing.T) {
			repo := newTestRepository(t)
			inventoryfixture.SeedInventory(
				t,
				repo,
				inventoryfixture.Row{Path: "/library/scope", RootPath: "/library", ContentRev: 8},
			)
			scope := "/library/scope"
			session := &inventory.ScanSession{
				SessionID: "scope",
				RootPath:  "/library",
				Kind:      kind,
				StartedAt: time.Now(),
			}
			if kind == "folder" {
				session.ScopePath = &scope
			}
			if err := repo.CreateScanSession(session); err != nil {
				t.Fatal(err)
			}
			stageSplitRows(
				t,
				repo,
				"scope",
				inventory.StagingEntry{Path: "/library/scope/new.mp3", Size: 10, Mtime: 20},
			)
			var err error
			if kind == "selected" {
				_, err = repo.MergeSelectedStaging(t.Context(), "scope", "/library", []string{scope})
			} else {
				_, err = repo.MergeStaging("scope", "", "/library")
			}
			if err != nil {
				t.Fatal(err)
			}
			assertSplitPaths(
				t,
				repo,
				map[string]string{"/library": "dir", "/library/scope": "dir", "/library/scope/new.mp3": "file"},
			)
		})
	}
}

func assertSplitPaths(t *testing.T, repo *Repository, want map[string]string) {
	t.Helper()
	rows, err := repo.DB().
		Query(`SELECT path,'',1 FROM inventory_dirs UNION ALL SELECT d.path,f.name,0 FROM inventory_files f JOIN inventory_dirs d ON d.id=f.dir_ref`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var p, name string
		var isDir bool
		if err := rows.Scan(&p, &name, &isDir); err != nil {
			t.Fatal(err)
		}
		kind := "dir"
		if !isDir {
			p = joinInventoryPath(p, name)
			kind = "file"
		}
		if _, ok := got[p]; ok {
			t.Fatalf("duplicate types at %s", p)
		}
		got[p] = kind
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stored paths=%v, want %v", got, want)
	}
}

func TestSplitMergeChangedFilesClearFactsAndAdvanceRevision(t *testing.T) {
	for _, tc := range []struct {
		name        string
		size, mtime int64
		format      string
		rev         int
		clear       bool
	}{
		{"size", 11, 20, "mp3", 8, true}, {"mtime", 10, 21, "mp3", 8, true}, {"format", 10, 20, "mpeg", 7, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newTestRepository(t)
			inventoryfixture.SeedInventory(
				t,
				repo,
				inventoryfixture.Row{
					Path:        "/library/track.mp3",
					RootPath:    "/library",
					Size:        10,
					Mtime:       20,
					Format:      "mp3",
					ContentRev:  7,
					Bitrate:     new(int32(320000)),
					Width:       new(30),
					Height:      new(40),
					DurationSec: new(float64(12)),
				},
			)
			stageSplitRows(
				t,
				repo,
				"changed",
				inventory.StagingEntry{Path: "/library/track.mp3", Size: tc.size, Mtime: tc.mtime, Format: tc.format},
			)
			if _, err := repo.MergeStaging("changed", "", "/library"); err != nil {
				t.Fatal(err)
			}
			rev, rate := splitFileRevision(t, repo, "/library", "track.mp3")
			var width, height sql.NullInt64
			var duration sql.NullFloat64
			if err := repo.DB().
				QueryRow("SELECT width,height,duration_sec FROM inventory_files WHERE name='track.mp3'").
				Scan(&width, &height, &duration); err != nil {
				t.Fatal(err)
			}
			if rev != tc.rev || rate.Valid == tc.clear || width.Valid == tc.clear || height.Valid == tc.clear ||
				duration.Valid == tc.clear {
				t.Fatalf("revision=%d rate=%v dimensions=%v/%v duration=%v", rev, rate, width, height, duration)
			}
		})
	}
}

func TestSplitMergeTypeReplacementAndScopedStaleCleanup(t *testing.T) {
	for _, kind := range []string{"full", "selected"} {
		t.Run(kind, func(t *testing.T) {
			repo := newTestRepository(t)
			inventoryfixture.SeedInventory(t, repo,
				inventoryfixture.Row{Path: "/library/a%_日", RootPath: "/library", IsDir: true, IsWorkRoot: true},
				inventoryfixture.Row{Path: "/library/a%_日/swap", RootPath: "/library", IsDir: true},
				inventoryfixture.Row{Path: "/library/a%_日/swap/child/old.mp3", RootPath: "/library", ContentRev: 9},
				inventoryfixture.Row{Path: "/library/a%_日/stale/deleted.mp3", RootPath: "/library"},
				inventoryfixture.Row{Path: "/library/a%_日2/neighbor.mp3", RootPath: "/library"},
			)
			merge := func(id string, rows ...inventory.StagingEntry) {
				t.Helper()
				if err := repo.CreateScanSession(
					&inventory.ScanSession{SessionID: id, RootPath: "/library", Kind: kind, StartedAt: time.Now()},
				); err != nil {
					t.Fatal(err)
				}
				if kind == "full" {
					rows = append(
						rows,
						inventory.StagingEntry{Path: "/library/a%_日", IsDir: true, IsWorkRoot: true},
						inventory.StagingEntry{Path: "/library/a%_日2", IsDir: true},
						inventory.StagingEntry{Path: "/library/a%_日2/neighbor.mp3"},
					)
				}
				stageSplitRows(t, repo, id, rows...)
				var err error
				if kind == "selected" {
					_, err = repo.MergeSelectedStaging(t.Context(), id, "/library", []string{"/library/a%_日"})
				} else {
					_, err = repo.MergeStaging(id, "", "/library")
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			merge("file", inventory.StagingEntry{Path: "/library/a%_日/swap", Size: 10, Mtime: 20})
			want := map[string]string{
				"/library":                    "dir",
				"/library/a%_日":               "dir",
				"/library/a%_日/swap":          "file",
				"/library/a%_日2":              "dir",
				"/library/a%_日2/neighbor.mp3": "file",
			}
			assertSplitPaths(t, repo, want)
			rev, _ := splitFileRevision(t, repo, "/library/a%_日", "swap")
			if rev != 1 {
				t.Fatalf("replacement revision=%d, want 1", rev)
			}
			merge(
				"dir",
				inventory.StagingEntry{Path: "/library/a%_日/swap", IsDir: true},
				inventory.StagingEntry{Path: "/library/a%_日/swap/new.mp3"},
			)
			want["/library/a%_日/swap"] = "dir"
			want["/library/a%_日/swap/new.mp3"] = "file"
			assertSplitPaths(t, repo, want)
		})
	}
}

func TestSplitMergeFailureRollsBackInventoryBindingsAndStaging(t *testing.T) {
	for _, kind := range []string{"full", "selected"} {
		t.Run(kind, func(t *testing.T) {
			repo := newTestRepository(t)
			library, err := repo.CreateLibrary("library", "/library")
			if err != nil {
				t.Fatal(err)
			}
			stageSplitRows(
				t,
				repo,
				"before",
				inventory.StagingEntry{Path: "/library/RJ000001", IsDir: true, IsWorkRoot: true},
				inventory.StagingEntry{Path: "/library/RJ000001/old.mp3", Size: 10, Mtime: 20},
			)
			if _, err = repo.MergeStaging("before", library.ID, "/library"); err != nil {
				t.Fatal(err)
			}
			if err = repo.CreateScanSession(
				&inventory.ScanSession{SessionID: "failed", RootPath: "/library", Kind: kind, StartedAt: time.Now()},
			); err != nil {
				t.Fatal(err)
			}
			stageSplitRows(
				t,
				repo,
				"failed",
				inventory.StagingEntry{Path: "/library/RJ000002", IsDir: true, IsWorkRoot: true},
				inventory.StagingEntry{Path: "/library/RJ000002/new.mp3"},
			)
			if _, err = repo.DB().
				Exec(`CREATE TRIGGER reject_split_cleanup BEFORE INSERT ON inventory_files BEGIN SELECT RAISE(ABORT,'cleanup failed'); END`); err != nil {
				t.Fatal(err)
			}
			if kind == "selected" {
				_, err = repo.MergeSelectedStaging(t.Context(), "failed", "/library", []string{"/library/RJ000002"})
			} else {
				_, err = repo.MergeStaging("failed", library.ID, "/library")
			}
			if err == nil {
				t.Fatal("expected merge failure")
			}
			assertSplitPaths(
				t,
				repo,
				map[string]string{"/library": "dir", "/library/RJ000001": "dir", "/library/RJ000001/old.mp3": "file"},
			)
			var bindings, staged int
			if err = repo.DB().
				QueryRow("SELECT COUNT(*) FROM work_copies WHERE library_id=? AND rel_path='RJ000001'", library.ID).
				Scan(&bindings); err != nil {
				t.Fatal(err)
			}
			if err = stagingTestRow(repo, "failed", "SELECT COUNT(*) FROM entries_staging").
				Scan(&staged); err != nil {
				t.Fatal(err)
			}
			if bindings != 1 || staged != 0 {
				t.Fatalf("bindings=%d staging=%d", bindings, staged)
			}
		})
	}
}

func TestSplitEmptyScansKeepLibraryAndScopeRoots(t *testing.T) {
	repo := newTestRepository(t)
	outcome, err := repo.MergeStaging("empty", "", "/library")
	if err != nil || outcome.Merged != 0 {
		t.Fatalf("empty merge=%+v, %v", outcome, err)
	}
	assertSplitPaths(t, repo, map[string]string{"/library": "dir"})
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/library/scope", RootPath: "/library", IsDir: true, IsWorkRoot: true},
		inventoryfixture.Row{Path: "/library/scope/stale.mp3", RootPath: "/library"},
	)
	if err = repo.CreateScanSession(
		&inventory.ScanSession{
			SessionID: "empty-selected",
			RootPath:  "/library",
			Kind:      "selected",
			StartedAt: time.Now(),
		},
	); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = repo.MergeSelectedStaging(ctx, "empty-selected", "/library", []string{"/library/scope"}); err == nil {
		t.Fatal("canceled merge accepted")
	}
	assertSplitPaths(
		t,
		repo,
		map[string]string{"/library": "dir", "/library/scope": "dir", "/library/scope/stale.mp3": "file"},
	)
	outcome, err = repo.MergeSelectedStaging(t.Context(), "empty-selected", "/library", []string{"/library/scope"})
	if err != nil || outcome.Merged != 0 {
		t.Fatalf("empty selected merge=%+v, %v", outcome, err)
	}
	assertSplitPaths(t, repo, map[string]string{"/library": "dir", "/library/scope": "dir"})
	var workRoot bool
	if err = repo.DB().
		QueryRow("SELECT is_work_root FROM inventory_dirs WHERE path='/library/scope'").
		Scan(&workRoot); err != nil ||
		!workRoot {
		t.Fatalf("scope work root=%v, %v", workRoot, err)
	}
}

func TestSplitMergePreservesFilesAtPOSIXRoots(t *testing.T) {
	for _, root := range []string{"/", "C:/", "//server/share", "/library"} {
		t.Run(root, func(t *testing.T) {
			repo := newTestRepository(t)
			file := joinInventoryPath(root, "track%_日.MP3")
			store := NewScanStaging(repo)
			for _, id := range []string{"first", "rescan"} {
				if err := store.WriteStagingEntries(
					id,
					[]inventory.StagingEntry{
						{
							SessionID:  id,
							RootPath:   root,
							Path:       file,
							ParentPath: root,
							Name:       "track%_日.MP3",
							Size:       10,
							Mtime:      20,
						},
					},
				); err != nil {
					t.Fatal(err)
				}
				outcome, err := repo.MergeStaging(id, "", root)
				if err != nil || outcome.Merged != 1 {
					t.Fatalf("merge=%+v, %v", outcome, err)
				}
			}
			assertSplitPaths(t, repo, map[string]string{root: "dir", file: "file"})
			rev, _ := splitFileRevision(t, repo, root, "track%_日.MP3")
			if rev != 1 {
				t.Fatalf("unchanged root file revision=%d", rev)
			}
		})
	}
}

func TestPackScopesNeverSplitsAndIsolatesOverBudgetScopes(t *testing.T) {
	scopes := []string{"a", "b", "c", "d", "e"}
	got := packScopes(scopes, []int{3, 3, 50, 1, 5}, 7)
	want := [][]string{{"a", "b"}, {"c"}, {"d", "e"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("batches = %v, want %v", got, want)
	}
}
