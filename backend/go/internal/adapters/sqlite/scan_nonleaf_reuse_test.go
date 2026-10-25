package sqlite_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/inventory"
)

func TestScanReusesNonleafFilesWhileObservingChildren(t *testing.T) {
	f := newScanTree(t)
	f.write("branch/track.mp3", "old")
	f.write("branch/child/removed.mp3", "removed")
	f.write("branch/Delete/hidden.mp3", "hidden")
	old := time.Now().Add(-time.Hour).Unix()
	setDirectoryMtime(t, f.root+"/branch", old)
	f.scan()
	if err := f.store.UpdateEntryMediaFacts([]inventory.MediaFactsUpdate{{
		Path: f.root + "/branch/track.mp3", Bitrate: 320000, DurationSec: 12, Width: 100, Height: 200,
	}}); err != nil {
		t.Fatal(err)
	}
	before := inventoryEntry(t, f.repo, f.root+"/branch/track.mp3")
	f.write("branch/track.mp3", "rewritten longer audio")
	f.write("branch/child/added.mp3", "added")
	if err := os.Remove(filepath.FromSlash(f.root + "/branch/child/removed.mp3")); err != nil {
		t.Fatal(err)
	}
	_, progress := f.scan()
	if got := inventoryEntry(t, f.repo, f.root+"/branch/track.mp3"); !reflect.DeepEqual(before, got) {
		t.Fatalf("nonleaf direct file was reread: before=%+v after=%+v", before, got)
	}
	inventoryEntry(t, f.repo, f.root+"/branch/child/added.mp3")
	if rows, err := f.repo.ListEntriesUnderPath(f.root + "/branch/child/removed.mp3"); err != nil || len(rows) != 0 {
		t.Fatalf("child deletion was hidden: %+v, %v", rows, err)
	}
	if progress.FilesScanned != 2 || progress.DirsScanned != 2 {
		t.Fatalf("progress=%+v", progress)
	}
}

func TestScanObservesAndReusesLibraryRoot(t *testing.T) {
	for _, nonleaf := range []bool{false, true} {
		t.Run(map[bool]string{false: "leaf", true: "nonleaf"}[nonleaf], func(t *testing.T) {
			f := newScanTree(t)
			f.root += "/RJ000009"
			f.write("track.mp3", "old")
			if nonleaf {
				f.write("RJ000001/child.mp3", "child")
			}
			old := time.Now().Add(-time.Hour).Unix()
			setDirectoryMtime(t, f.root, old)
			_, progress := f.scan()
			roots, err := f.repo.ListLibraryDirs(f.root)
			expected := 0
			if nonleaf {
				expected = 1
			}
			if err != nil || len(roots) != expected {
				t.Fatalf("root became a work: %+v, %v", roots, err)
			}
			before := inventoryEntry(t, f.repo, f.root+"/track.mp3")
			f.write("track.mp3", "rewritten longer audio")
			_, carried := f.scan()
			if got := inventoryEntry(t, f.repo, f.root+"/track.mp3"); !reflect.DeepEqual(before, got) {
				t.Fatalf("library root files were reread: %+v", got)
			}
			if !reflect.DeepEqual(carried, progress) {
				t.Fatalf("root reuse changed progress: before=%+v after=%+v", progress, carried)
			}
			f.scan(inventory.WithFullScan())
			if got := inventoryEntry(t, f.repo, f.root+"/track.mp3"); got.Size != 22 || got.ContentRev != 2 {
				t.Fatalf("full rescan did not observe root file: %+v", got)
			}
		})
	}
}

func TestScanNonleafEntryChangesOverrideCachedMtime(t *testing.T) {
	for _, atRoot := range []bool{false, true} {
		for _, mutation := range []string{"create", "delete", "rename"} {
			t.Run(map[bool]string{false: "branch", true: "root"}[atRoot]+"/"+mutation, func(t *testing.T) {
				f := newScanTree(t)
				dir := "branch/"
				if atRoot {
					dir = ""
				}
				f.write(dir+"child/kept.mp3", "child")
				f.write(dir+"track.mp3", "old")
				f.write(dir+"changed.mp3", "changed")
				old := time.Now().Add(-time.Hour).Unix()
				setDirectoryMtime(t, f.root+"/"+dir, old)
				f.scan()
				// A listing mismatch must observe every direct file, including unchanged names.
				f.write(dir+"track.mp3", "rewritten longer audio")
				changed := filepath.FromSlash(f.root + "/" + dir + "changed.mp3")
				var err error
				switch mutation {
				case "create":
					f.write(dir+"added.mp3", "added")
				case "delete":
					err = os.Remove(changed)
				case "rename":
					err = os.Rename(changed, filepath.FromSlash(f.root+"/"+dir+"renamed.mp3"))
				}
				if err != nil {
					t.Fatal(err)
				}
				setDirectoryMtime(t, f.root+"/"+dir, old)
				f.scan()
				if got := inventoryEntry(t, f.repo, f.root+"/"+dir+"track.mp3"); got.Size != 22 || got.ContentRev != 2 {
					t.Fatalf("listing mismatch did not observe all files: %+v", got)
				}
				ordinary := f.entries()
				f.scan(inventory.WithFullScan())
				if full := f.entries(); !reflect.DeepEqual(ordinary, full) {
					t.Fatalf("ordinary=%+v full=%+v", ordinary, full)
				}
			})
		}
	}
}

func TestScanNonleafSameNameReplacementWaitsForFreshMtime(t *testing.T) {
	f := newScanTree(t)
	f.write("branch/child/kept.mp3", "child")
	f.write("branch/track.mp3", "old")
	old := time.Now().Add(-time.Hour).Unix()
	setDirectoryMtime(t, f.root+"/branch", old)
	f.scan()
	before := inventoryEntry(t, f.repo, f.root+"/branch/track.mp3")
	replacement := filepath.Join(t.TempDir(), "replacement.mp3")
	if err := os.WriteFile(replacement, []byte("replacement bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, filepath.FromSlash(f.root+"/branch/track.mp3")); err != nil {
		t.Fatal(err)
	}
	setDirectoryMtime(t, f.root+"/branch", old)
	f.scan()
	if got := inventoryEntry(t, f.repo, f.root+"/branch/track.mp3"); !reflect.DeepEqual(before, got) {
		t.Fatalf("same-name replacement with stale attributes was reread: %+v", got)
	}
	setDirectoryMtime(t, f.root+"/branch", old+1)
	f.scan()
	if got := inventoryEntry(t, f.repo, f.root+"/branch/track.mp3"); got.Size != 17 || got.ContentRev != 2 {
		t.Fatalf("fresh directory mtime did not reveal replacement: %+v", got)
	}
}

func TestScanNonleafReuseTreatsDirectorySymlinkAsFile(t *testing.T) {
	f := newScanTree(t)
	f.write("branch/child/track.mp3", "child")
	f.write("branch/track.mp3", "old")
	if err := os.Symlink(
		filepath.FromSlash(f.root+"/branch/child"),
		filepath.FromSlash(f.root+"/branch/link"),
	); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	setDirectoryMtime(t, f.root+"/branch", time.Now().Add(-time.Hour).Unix())
	f.scan()
	before := inventoryEntry(t, f.repo, f.root+"/branch/track.mp3")
	link := inventoryEntry(t, f.repo, f.root+"/branch/link")
	if link.IsDir {
		t.Fatalf("symlink became a directory: %+v", link)
	}
	f.write("branch/track.mp3", "rewritten longer audio")
	f.scan()
	if got := inventoryEntry(t, f.repo, f.root+"/branch/track.mp3"); !reflect.DeepEqual(before, got) {
		t.Fatalf("symlink caused false listing mismatch: %+v", got)
	}
	if got := inventoryEntry(t, f.repo, f.root+"/branch/link"); !reflect.DeepEqual(link, got) {
		t.Fatalf("carried symlink changed: %+v", got)
	}
}
