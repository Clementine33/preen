package sqlite_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"
)

type scanTree struct {
	t        *testing.T
	root     string
	repo     *sqlite.Repository
	store    *sqlite.ScanStaging
	pipeline *inventory.Pipeline
}

func newScanTree(t *testing.T) *scanTree {
	t.Helper()
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	store := sqlite.NewScanStaging(repo)
	return &scanTree{t: t, root: filepath.ToSlash(t.TempDir()), repo: repo, store: store,
		pipeline: inventory.NewPipeline(store, filesystem.WalkRootEntriesParallel, filesystem.WalkFolderEntries, nil)}
}

func (f *scanTree) write(path, contents string) {
	f.t.Helper()
	full := filepath.FromSlash(f.root + "/" + path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *scanTree) scan(opts ...inventory.ScanOption) (string, inventory.Progress) {
	f.t.Helper()
	var progress inventory.Progress
	opts = append(opts, inventory.WithProgress(func(p inventory.Progress) { progress = p }))
	id, err := f.pipeline.ScanRootCtx(f.t.Context(), f.root, opts...)
	if err != nil {
		f.t.Fatal(err)
	}
	return id, progress
}

func (f *scanTree) entries() []inventory.Entry {
	f.t.Helper()
	rows, err := f.repo.ListEntriesUnderPath(f.root)
	if err != nil {
		f.t.Fatal(err)
	}
	return rows
}

func TestScanReusesUnchangedLeafFiles(t *testing.T) {
	f := newScanTree(t)
	f.write("RJ000001/track.mp3", "audio")
	f.write("classification/leaf/other.txt", "other")
	old := time.Now().Add(-time.Hour).Unix()
	setDirectoryMtime(t, f.root+"/RJ000001", old)
	setDirectoryMtime(t, f.root+"/classification/leaf", old)
	f.scan()
	if err := f.store.UpdateEntryMediaFacts(
		[]inventory.MediaFactsUpdate{
			{Path: f.root + "/RJ000001/track.mp3", Bitrate: 320000, DurationSec: 12, Width: 100, Height: 200},
		},
	); err != nil {
		t.Fatal(err)
	}
	before := f.entries()
	_, progress := f.scan()
	if after := f.entries(); !reflect.DeepEqual(before, after) || progress.FilesScanned != 2 ||
		progress.DirsScanned != 3 {
		t.Fatalf("unchanged scan=%+v progress=%+v; before=%+v", after, progress, before)
	}
	f.write("RJ000001/track.mp3", "rewritten longer audio")
	f.scan()
	if after := f.entries(); !reflect.DeepEqual(before, after) {
		t.Fatalf("ordinary scan read carried files: before=%+v after=%+v", before, after)
	}
	f.scan(inventory.WithFullScan())
	updated := inventoryEntry(t, f.repo, f.root+"/RJ000001/track.mp3")
	if updated.Size != 22 || updated.ContentRev != 2 || updated.Bitrate != nil || updated.DurationSec != nil {
		t.Fatalf("full observation did not refresh content: %+v", updated)
	}
}

func TestScanReuseKeepsDeepChangesVisible(t *testing.T) {
	f := newScanTree(t)
	f.write("branch/deep/kept/track.mp3", "kept")
	f.write("branch/deep/removed/track.mp3", "removed")
	old := time.Now().Add(-time.Hour).Unix()
	for _, rel := range []string{"branch", "branch/deep", "branch/deep/kept", "branch/deep/removed"} {
		setDirectoryMtime(t, f.root+"/"+rel, old)
	}
	f.scan()
	f.write("branch/deep/kept/new.mp3", "new")
	f.write("branch/deep/added/track.mp3", "added")
	if err := os.RemoveAll(filepath.FromSlash(f.root + "/branch/deep/removed")); err != nil {
		t.Fatal(err)
	}
	f.scan()
	inventoryEntry(t, f.repo, f.root+"/branch/deep/kept/new.mp3")
	inventoryEntry(t, f.repo, f.root+"/branch/deep/added/track.mp3")
	if rows, err := f.repo.ListEntriesUnderPath(f.root + "/branch/deep/removed"); err != nil || len(rows) != 0 {
		t.Fatalf("deleted subtree survived: %+v, %v", rows, err)
	}
	ordinary := f.entries()
	f.scan(inventory.WithFullScan())
	if full := f.entries(); !reflect.DeepEqual(ordinary, full) {
		t.Fatalf("deep changes ordinary=%+v full=%+v", ordinary, full)
	}
}

func TestScanReuseReconcilesFileDirectorySwaps(t *testing.T) {
	for _, directoryFirst := range []bool{false, true} {
		t.Run(
			map[bool]string{true: "directory to file", false: "file to directory"}[directoryFirst],
			func(t *testing.T) {
				f := newScanTree(t)
				f.write("branch/sibling/kept.mp3", "kept")
				f.write("branch/direct.mp3", "old")
				if directoryFirst {
					f.write("branch/item/old.mp3", "old")
				} else {
					f.write("branch/item", "old")
				}
				old := time.Now().Add(-time.Hour).Unix()
				setDirectoryMtime(t, f.root+"/branch", old)
				if directoryFirst {
					setDirectoryMtime(t, f.root+"/branch/item", old)
				}
				f.scan()
				if err := os.RemoveAll(filepath.FromSlash(f.root + "/branch/item")); err != nil {
					t.Fatal(err)
				}
				if directoryFirst {
					f.write("branch/item", "new file")
				} else {
					f.write("branch/item/new.mp3", "new")
				}
				f.write("branch/direct.mp3", "rewritten longer audio")
				setDirectoryMtime(t, f.root+"/branch", old)
				f.scan()
				if direct := inventoryEntry(t, f.repo, f.root+"/branch/direct.mp3"); direct.Size != 22 {
					t.Fatalf("type mismatch did not observe all direct files: %+v", direct)
				}
				got := inventoryEntry(t, f.repo, f.root+"/branch/item")
				if got.IsDir == directoryFirst {
					t.Fatalf("wrong replacement type: %+v", got)
				}
				if directoryFirst {
					if rows, err := f.repo.ListEntriesUnderPath(f.root + "/branch/item"); err != nil || len(rows) != 1 {
						t.Fatalf("stale descendants: %+v, %v", rows, err)
					}
				} else {
					inventoryEntry(t, f.repo, f.root+"/branch/item/new.mp3")
				}
				ordinary := f.entries()
				f.scan(inventory.WithFullScan())
				if full := f.entries(); !reflect.DeepEqual(ordinary, full) {
					t.Fatalf("type swap ordinary=%+v full=%+v", ordinary, full)
				}
			},
		)
	}
}

func TestScanReuseKeepsWorkBindingsAndCollectionTime(t *testing.T) {
	f := newScanTree(t)
	lib, err := f.repo.CreateLibrary("test", f.root)
	if err != nil {
		t.Fatal(err)
	}
	f.write("classification/RJ000001/track.mp3", "audio")
	old := time.Now().Add(-time.Hour).Unix()
	setDirectoryMtime(t, f.root+"/classification/RJ000001", old)
	f.scan(inventory.WithLibrary(lib.ID))
	before, err := f.repo.ListLibraryDirs(f.root)
	if err != nil || len(before) != 1 {
		t.Fatalf("work roots=%+v, %v", before, err)
	}
	paths := []string{f.root + "/classification/RJ000001"}
	bindings, err := f.repo.CoverTargets(t.Context(), f.root, paths)
	if err != nil || bindings[paths[0]].CopyID == "" {
		t.Fatalf("binding=%+v, %v", bindings, err)
	}
	f.scan(inventory.WithLibrary(lib.ID))
	after, err := f.repo.ListLibraryDirs(f.root)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("carried work collection changed: %+v, %v", after, err)
	}
	afterBindings, err := f.repo.CoverTargets(t.Context(), f.root, paths)
	if err != nil || !reflect.DeepEqual(bindings, afterBindings) {
		t.Fatalf("carried work binding changed: %+v, %v", afterBindings, err)
	}
	f.write("classification/RJ000002/track.mp3", "new work")
	f.scan(inventory.WithLibrary(lib.ID))
	if roots, err := f.repo.ListLibraryWorkRoots(
		f.root,
	); err != nil ||
		!reflect.DeepEqual(roots, []string{"classification/RJ000001", "classification/RJ000002"}) {
		t.Fatalf("added work roots=%v, %v", roots, err)
	}
	if err := os.RemoveAll(filepath.FromSlash(paths[0])); err != nil {
		t.Fatal(err)
	}
	f.scan(inventory.WithLibrary(lib.ID))
	if roots, err := f.repo.ListLibraryWorkRoots(
		f.root,
	); err != nil ||
		!reflect.DeepEqual(roots, []string{"classification/RJ000002"}) {
		t.Fatalf("removed work roots=%v, %v", roots, err)
	}
	if bindings, err := f.repo.CoverTargets(t.Context(), f.root, paths); err != nil || bindings[paths[0]].CopyID != "" {
		t.Fatalf("removed binding=%+v, %v", bindings, err)
	}
}

func TestScanReuseSurvivesSubtreeMovesAndExcludesRecovery(t *testing.T) {
	for _, relocateInventory := range []bool{false, true} {
		t.Run(
			map[bool]string{true: "inventory relocation", false: "external relocation"}[relocateInventory],
			func(t *testing.T) {
				f := newScanTree(t)
				f.write("old/RJ000001/track.mp3", "audio")
				f.write("classification/existing.txt", "existing")
				f.write("Delete/RJ000002/hidden.mp3", "hidden")
				f.write("classification/Delete/RJ000003/hidden.mp3", "hidden")
				setDirectoryMtime(t, f.root+"/old/RJ000001", time.Now().Add(-time.Hour).Unix())
				f.scan()
				source, target := f.root+"/old", f.root+"/classification/moved"
				if relocateInventory {
					target = f.root + "/renamed"
				}
				if err := os.Rename(filepath.FromSlash(source), filepath.FromSlash(target)); err != nil {
					t.Fatal(err)
				}
				if relocateInventory {
					if err := f.repo.RelocateDirectory(t.Context(), f.root, source, target); err != nil {
						t.Fatal(err)
					}
				}
				f.scan()
				inventoryEntry(t, f.repo, target+"/RJ000001/track.mp3")
				for _, path := range []string{source, f.root + "/Delete", f.root + "/classification/Delete"} {
					if rows, err := f.repo.ListEntriesUnderPath(path); err != nil || len(rows) != 0 {
						t.Fatalf("excluded subtree %s=%+v, %v", path, rows, err)
					}
				}
				ordinary := f.entries()
				f.scan(inventory.WithFullScan())
				if full := f.entries(); !reflect.DeepEqual(ordinary, full) {
					t.Fatalf("moved subtree ordinary=%+v full=%+v", ordinary, full)
				}
			},
		)
	}
}

func TestScopedScansReadFilesEvenInReusableLeaves(t *testing.T) {
	for _, selected := range []bool{false, true} {
		t.Run(map[bool]string{true: "selected", false: "member"}[selected], func(t *testing.T) {
			f := newScanTree(t)
			f.write("RJ000001/leaf/track.mp3", "old")
			setDirectoryMtime(t, f.root+"/RJ000001/leaf", time.Now().Add(-time.Hour).Unix())
			f.scan()
			f.write("RJ000001/leaf/track.mp3", "new longer bytes")
			var err error
			if selected {
				_, err = f.pipeline.ScanFoldersCtx(t.Context(), []string{f.root + "/RJ000001"}, f.root, 1)
			} else {
				_, err = f.pipeline.ScanFolderCtx(t.Context(), f.root+"/RJ000001", f.root)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := inventoryEntry(
				t,
				f.repo,
				f.root+"/RJ000001/leaf/track.mp3",
			); got.Size != 16 ||
				got.ContentRev != 2 {
				t.Fatalf("scoped refresh reused leaf: %+v", got)
			}
		})
	}
}

func TestScanDoesNotReuseRecentOrZeroDirectoryMtimes(t *testing.T) {
	for _, offset := range []int64{0, -5, -9, -11} {
		t.Run((time.Duration(offset) * time.Second).String(), func(t *testing.T) {
			f := newScanTree(t)
			f.write("leaf/track.mp3", "old")
			stamp := int64(0)
			if offset != 0 {
				stamp = time.Now().Unix() + offset
			}
			setDirectoryMtime(t, f.root+"/leaf", stamp)
			f.scan()
			f.write("leaf/track.mp3", "new longer bytes")
			f.scan()
			got := inventoryEntry(t, f.repo, f.root+"/leaf/track.mp3")
			wantSize := int64(16)
			if offset == -11 {
				wantSize = 3
			}
			if got.Size != wantSize {
				t.Fatalf("mtime offset %d: %+v", offset, got)
			}
		})
	}
}

func TestScanReuseStrictTenSecondBoundary(t *testing.T) {
	f := newScanTree(t)
	f.write("leaf/track.mp3", "new longer bytes")
	old := time.Now().Add(-time.Hour).Unix()
	setDirectoryMtime(t, f.root+"/leaf", old)
	inventoryfixture.SeedInventory(t, f.repo,
		inventoryfixture.Row{Path: f.root + "/leaf", RootPath: f.root, IsDir: true, Mtime: old},
		inventoryfixture.Row{Path: f.root + "/leaf/track.mp3", RootPath: f.root, Size: 3, ContentRev: 1},
	)
	if err := f.repo.CreateScanSession(
		&inventory.ScanSession{
			SessionID: "boundary",
			RootPath:  f.root,
			Kind:      "full",
			Status:    "completed",
			StartedAt: time.Unix(old+10, 0),
		},
	); err != nil {
		t.Fatal(err)
	}
	f.scan()
	if got := inventoryEntry(t, f.repo, f.root+"/leaf/track.mp3"); got.Size != 16 || got.ContentRev != 2 {
		t.Fatalf("boundary directory was reused: %+v", got)
	}
}

func TestScanRequiresRetainedSuccessfulRootScanForReuse(t *testing.T) {
	for _, scenario := range []string{"first scan", "retention", "other root", "folder", "selected", "failed"} {
		t.Run(scenario, func(t *testing.T) {
			f := newScanTree(t)
			f.write("leaf/track.mp3", "old")
			old := time.Now().Add(-time.Hour).Unix()
			setDirectoryMtime(t, f.root+"/leaf", old)
			if scenario == "retention" {
				f.scan()
				stats, err := f.repo.RunRetentionCleanupBatch(t.Context(), time.Now().Add(time.Hour), time.Time{}, 100)
				if err != nil || stats.DeletedScanSessions != 1 {
					t.Fatalf("retention=%+v, %v", stats, err)
				}
			} else {
				inventoryfixture.SeedInventory(t, f.repo,
					inventoryfixture.Row{Path: f.root + "/leaf", RootPath: f.root, IsDir: true, Mtime: old},
					inventoryfixture.Row{Path: f.root + "/leaf/track.mp3", RootPath: f.root, Size: 3, ContentRev: 1},
				)
				if scenario != "first scan" {
					session := &inventory.ScanSession{
						SessionID: "ineligible",
						RootPath:  f.root,
						Kind:      "full",
						Status:    "completed",
						StartedAt: time.Now(),
					}
					switch scenario {
					case "other root":
						session.RootPath += "-other"
					case "folder", "selected":
						session.Kind = scenario
						session.ScopePath = new(f.root + "/leaf")
					case "failed":
						session.Status = "failed"
					}
					if err := f.repo.CreateScanSession(session); err != nil {
						t.Fatal(err)
					}
				}
			}
			f.write("leaf/track.mp3", "new longer bytes")
			f.scan()
			if got := inventoryEntry(t, f.repo, f.root+"/leaf/track.mp3"); got.Size != 16 || got.ContentRev != 2 {
				t.Fatalf("without retained success=%+v", got)
			}
		})
	}
}

func TestScanReadsSyntheticDirectoryAfterExecution(t *testing.T) {
	f := newScanTree(t)
	f.scan()
	f.write("created/track.mp3", "new longer bytes")
	setDirectoryMtime(t, f.root+"/created", time.Now().Add(-time.Hour).Unix())
	if err := f.repo.SyncObservedInventory(
		f.root,
		nil,
		[]inventory.InventoryFile{{Path: f.root + "/created/track.mp3", Size: 3, Mtime: 1}},
		nil,
	); err != nil {
		t.Fatal(err)
	}
	if got := inventoryEntry(t, f.repo, f.root+"/created"); got.Mtime != 0 {
		t.Fatalf("fixture ancestor=%+v", got)
	}
	f.scan()
	if got := inventoryEntry(t, f.repo, f.root+"/created/track.mp3"); got.Size != 16 || got.ContentRev != 2 {
		t.Fatalf("synthetic directory was reused: %+v", got)
	}
}

func TestScanLeafMutationsMatchFullObservation(t *testing.T) {
	for _, mutation := range []string{"create", "delete", "rename", "replace"} {
		t.Run(mutation, func(t *testing.T) {
			f := newScanTree(t)
			f.write("leaf/kept.mp3", "kept")
			f.write("leaf/changed.mp3", "old")
			setDirectoryMtime(t, f.root+"/leaf", time.Now().Add(-time.Hour).Unix())
			f.scan()
			if err := f.store.UpdateEntryMediaFacts(
				[]inventory.MediaFactsUpdate{{Path: f.root + "/leaf/kept.mp3", Bitrate: 320000, DurationSec: 12}},
			); err != nil {
				t.Fatal(err)
			}
			kept := inventoryEntry(t, f.repo, f.root+"/leaf/kept.mp3")
			path := filepath.FromSlash(f.root + "/leaf/changed.mp3")
			var err error
			switch mutation {
			case "create":
				f.write("leaf/new.mp3", "new")
			case "delete":
				err = os.Remove(path)
			case "rename":
				err = os.Rename(path, filepath.FromSlash(f.root+"/leaf/renamed.mp3"))
			case "replace":
				f.write("replacement.mp3", "replacement bytes")
				err = os.Rename(filepath.FromSlash(f.root+"/replacement.mp3"), path)
			}
			if err != nil {
				t.Fatal(err)
			}
			_, progress := f.scan()
			if got := inventoryEntry(t, f.repo, f.root+"/leaf/kept.mp3"); !reflect.DeepEqual(kept, got) {
				t.Fatalf("unchanged media facts lost: %+v", got)
			}
			if mutation == "replace" {
				if got := inventoryEntry(t, f.repo, f.root+"/leaf/changed.mp3"); got.Size != 17 || got.ContentRev != 2 {
					t.Fatalf("replacement=%+v", got)
				}
			}
			ordinary := f.entries()
			wantFiles := map[string]int{"create": 3, "delete": 1, "rename": 2, "replace": 2}[mutation]
			if progress.FilesScanned != wantFiles {
				t.Fatalf("mutation progress=%+v, want %d files", progress, wantFiles)
			}
			f.scan(inventory.WithFullScan())
			if full := f.entries(); !reflect.DeepEqual(ordinary, full) {
				t.Fatalf("ordinary=%+v full=%+v", ordinary, full)
			}
		})
	}
}

func TestScanReuseWaitsForFreshDirectoryAttributes(t *testing.T) {
	f := newScanTree(t)
	f.write("leaf/kept.mp3", "kept")
	old := time.Now().Add(-time.Hour).Unix()
	setDirectoryMtime(t, f.root+"/leaf", old)
	f.scan()
	f.write("leaf/new.mp3", "new")
	setDirectoryMtime(t, f.root+"/leaf", old)
	_, progress := f.scan()
	if len(f.entries()) != 2 || progress.FilesScanned != 1 {
		t.Fatalf("cached observation was not carried: %+v %+v", f.entries(), progress)
	}
	setDirectoryMtime(t, f.root+"/leaf", time.Now().Unix())
	f.scan()
	inventoryEntry(t, f.repo, f.root+"/leaf/new.mp3")
}
