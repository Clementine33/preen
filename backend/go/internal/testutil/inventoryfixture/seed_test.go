package inventoryfixture_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"
)

func TestSeedInventoryCreatesAncestorsAndPreservesStoredFacts(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "inventory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	inventoryfixture.SeedInventory(t, repo, inventoryfixture.Row{
		Path:        "/music/work/disc/track.mp3",
		RootPath:    "/music",
		Size:        12,
		Mtime:       34,
		ContentRev:  7,
		Bitrate:     new(int32(320000)),
		DurationSec: new(float64(8)),
	},
		inventoryfixture.Row{
			Path:       "/music/work",
			RootPath:   "/music",
			IsDir:      true,
			IsWorkRoot: true,
			CreatedAt:  1767323045,
		})
	entries, err := repo.ListEntriesUnderPath("/music/work")
	if err != nil || len(entries) != 3 {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
	if !entries[0].IsDir || !entries[1].IsDir || entries[1].Path != "/music/work/disc" {
		t.Fatalf("ancestors=%+v", entries)
	}
	file, found, err := repo.EntryByPath("/music/work/disc/track.mp3")
	if err != nil || !found || file.Size != 12 || file.Mtime != 34 || file.ContentRev != 7 || file.Bitrate == nil ||
		*file.Bitrate != 320000 ||
		file.DurationSec == nil ||
		*file.DurationSec != 8 {
		t.Fatalf("file=%+v found=%v err=%v", file, found, err)
	}
	dirs, err := repo.ListLibraryDirs("/music")
	if err != nil || len(dirs) != 1 || dirs[0].AddedAt.Format("2006-01-02") != "2026-01-02" {
		t.Fatalf("dirs=%+v err=%v", dirs, err)
	}
}

func TestSeedInventoryKeepsPOSIXRootForms(t *testing.T) {
	for _, tc := range []struct{ root, parent, file string }{
		{"/", "/work/disc", "/work/disc/cover.png"},
		{"C:/", "C:/work/disc", "C:/work/disc/cover.png"},
		{"//server/share", "//server/share/work/disc", "//server/share/work/disc/cover.png"},
		{"/music", "/music/work/disc", "/music/work/disc/cover.png"},
	} {
		t.Run(tc.root, func(t *testing.T) {
			repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "inventory.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repo.Close() })
			inventoryfixture.SeedInventory(t, repo, inventoryfixture.Row{
				Path: tc.file, RootPath: tc.root, Format: "image/png", Width: new(10), Height: new(20),
			})
			entry, found, err := repo.EntryByPath(tc.file)
			if err != nil || !found || entry.ParentPath != tc.parent || entry.Name != "cover.png" ||
				entry.ContentRev != 1 ||
				entry.Format != "image/png" ||
				entry.Width == nil ||
				*entry.Width != 10 ||
				entry.Height == nil ||
				*entry.Height != 20 {
				t.Fatalf("entry=%+v found=%v err=%v", entry, found, err)
			}
			entries, err := repo.ListEntriesUnderPath(tc.parent)
			if err != nil || len(entries) != 2 || !entries[0].IsDir {
				t.Fatalf("entries=%+v err=%v", entries, err)
			}
		})
	}
}

func TestClearBitratesPreservesOtherFactsAndDirectories(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "inventory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	inventoryfixture.SeedInventory(t, repo, inventoryfixture.Row{
		Path:       "/music/work",
		RootPath:   "/music",
		IsDir:      true,
		IsWorkRoot: true,
		Bitrate:    new(int32(9)),
	},
		inventoryfixture.Row{
			Path:        "/music/work/track.mp3",
			RootPath:    "/music",
			Size:        12,
			Mtime:       34,
			ContentRev:  7,
			Bitrate:     new(int32(320000)),
			Width:       new(10),
			Height:      new(20),
			DurationSec: new(8.5),
		})
	before, err := repo.ListEntriesUnderPath("/music")
	if err != nil {
		t.Fatal(err)
	}
	inventoryfixture.ClearBitrates(t, repo)
	after, err := repo.ListEntriesUnderPath("/music")
	if err != nil {
		t.Fatal(err)
	}
	before[1].Bitrate = nil
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("facts changed: before=%+v after=%+v", before, after)
	}
	dirs, err := repo.ListLibraryDirs("/music")
	if err != nil || len(dirs) != 1 {
		t.Fatalf("work roots changed: %+v %v", dirs, err)
	}
}
