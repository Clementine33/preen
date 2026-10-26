package sqlite_test

import (
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

func TestExactSyncMakesNewCoverAMediaFactsCandidateWithoutAScan(t *testing.T) {
	repo := newExecutionRepo(t)
	const cover = "/music/album/cover.jpg"
	if err := repo.SyncObservedInventory("/music", nil, []inventory.InventoryFile{
		{Path: cover, Size: 200, Mtime: 1000},
	}, nil); err != nil {
		t.Fatal(err)
	}
	entry := inventoryEntry(t, repo, cover)
	if entry.Format != "image/jpeg" || entry.ContentRev != 1 {
		t.Fatalf("new cover facts = %+v", entry)
	}
	candidates, err := sqlite.NewScanStaging(repo).EntriesMissingMediaFacts("", 10)
	if err != nil || len(candidates) != 1 || candidates[0].Path != cover || candidates[0].Format != "image/jpeg" {
		t.Fatalf("media candidates = %+v, err = %v", candidates, err)
	}
}

func TestExactSyncInvalidatesOnlyChangedMediaFacts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		file    string
		size    int64
		mtime   int64
		changed bool
	}{
		{name: "cover size changed", file: "cover.jpg", size: 201, mtime: 1000, changed: true},
		{name: "cover mtime changed", file: "cover.jpg", size: 200, mtime: 1001, changed: true},
		{name: "cover unchanged", file: "cover.jpg", size: 200, mtime: 1000},
		{name: "audio changed", file: "track.mp3", size: 201, mtime: 1001, changed: true},
		{name: "audio unchanged", file: "track.mp3", size: 200, mtime: 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newExecutionRepo(t)
			path := "/music/album/" + tc.file
			if err := repo.SyncObservedInventory("/music", nil, []inventory.InventoryFile{
				{Path: path, Size: 200, Mtime: 1000},
			}, nil); err != nil {
				t.Fatal(err)
			}
			store := sqlite.NewScanStaging(repo)
			if err := store.UpdateEntryMediaFacts([]inventory.MediaFactsUpdate{
				{Path: path, Width: 640, Height: 480, DurationSec: 12.5, Bitrate: 320000},
			}); err != nil {
				t.Fatal(err)
			}
			if err := repo.SyncObservedInventory("/music", nil, []inventory.InventoryFile{
				{Path: path, Size: tc.size, Mtime: tc.mtime},
			}, nil); err != nil {
				t.Fatal(err)
			}
			entry := inventoryEntry(t, repo, path)
			if entry.Size != tc.size || entry.Mtime != tc.mtime || entry.Format == "" {
				t.Fatalf("synced file = %+v", entry)
			}
			if tc.changed {
				if entry.ContentRev != 2 || entry.Width != nil || entry.Height != nil || entry.DurationSec != nil ||
					entry.Bitrate != nil {
					t.Fatalf("changed file retained stale facts = %+v", entry)
				}
			} else if entry.ContentRev != 1 || entry.Width == nil || *entry.Width != 640 || entry.Height == nil || *entry.Height != 480 ||
				entry.DurationSec == nil || *entry.DurationSec != 12.5 || entry.Bitrate == nil || *entry.Bitrate != 320000 {
				t.Fatalf("unchanged file lost measured facts = %+v", entry)
			}
			candidates, err := store.EntriesMissingMediaFacts("", 10)
			if err != nil || (tc.changed && (len(candidates) != 1 || candidates[0].Path != path)) ||
				(!tc.changed && len(candidates) != 0) {
				t.Fatalf("media candidates = %+v, err = %v", candidates, err)
			}
		})
	}
}

func TestExactSyncWakesOnlyForCommittedFileChanges(t *testing.T) {
	repo := newExecutionRepo(t)
	wakes := 0
	repo.OnMediaFactsNeeded = func() { wakes++ }
	file := inventory.InventoryFile{Path: "/music/cover.jpg", Size: 200, Mtime: 1000}
	for i := range 2 {
		if err := repo.SyncObservedInventory("/music", nil, []inventory.InventoryFile{file}, nil); err != nil {
			t.Fatal(err)
		}
		if wakes != 1 {
			t.Fatalf("sync %d: wakes = %d, want 1", i, wakes)
		}
	}
	file.Mtime++
	if err := repo.SyncObservedInventory("/music", nil, []inventory.InventoryFile{file}, nil); err != nil {
		t.Fatal(err)
	}
	if wakes != 2 {
		t.Fatalf("changed file: wakes = %d, want 2", wakes)
	}
	if err := repo.SyncObservedInventory("/music", []string{file.Path}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if wakes != 2 {
		t.Fatalf("deletion only: wakes = %d, want 2", wakes)
	}
	if _, err := repo.DB().Exec(`CREATE TRIGGER reject_sync BEFORE INSERT ON inventory_files
		WHEN NEW.name='fail.jpg' BEGIN SELECT RAISE(ABORT, 'sync failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := repo.SyncObservedInventory("/music", nil, []inventory.InventoryFile{
		file, {Path: "/music/fail.jpg", Size: 200, Mtime: 1000},
	}, nil); err == nil {
		t.Fatal("expected sync failure")
	}
	if wakes != 2 {
		t.Fatalf("rolled back sync: wakes = %d, want 2", wakes)
	}
	if _, found, err := repo.EntryByPath(file.Path); err != nil || found {
		t.Fatalf("rolled back file: found = %v, err = %v", found, err)
	}
}
