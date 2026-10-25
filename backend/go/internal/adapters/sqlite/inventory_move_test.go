package sqlite_test

import (
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"
)

func TestDirectInventoryMovesPreserveFactsAndUpdateAudioCounts(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/lib/work", RootPath: "/lib", IsDir: true, IsWorkRoot: true},
	)
	inventoryfixture.SeedInventory(t, repo, inventoryfixture.Row{Path: "/lib/work/to", RootPath: "/lib", IsDir: true})
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/lib/work/track.mp3", RootPath: "/lib", Size: 100, Mtime: 3, ContentRev: 4},
	)
	if err := sqlite.NewScanStaging(repo).
		UpdateEntryMediaFacts([]inventory.MediaFactsUpdate{{Path: "/lib/work/track.mp3", Bitrate: 320000, DurationSec: 12}}); err != nil {
		t.Fatal(err)
	}
	for _, move := range [][2]string{{"/lib/work/track.mp3", "/lib/work/track.txt"}, {"/lib/work/track.txt", "/lib/work/to/track.txt"}} {
		if err := repo.MoveInventoryPath(t.Context(), "/lib", move[0], move[1]); err != nil {
			t.Fatal(err)
		}
		if found, err := repo.RootExistsInInventory(move[0]); err != nil || found {
			t.Fatalf("old path %s: %v %v", move[0], found, err)
		}
		got, found, err := repo.EntryByPath(move[1])
		if err != nil || !found || got.ContentRev != 4 || got.Bitrate == nil || *got.Bitrate != 320000 ||
			got.DurationSec == nil ||
			*got.DurationSec != 12 {
			t.Fatalf("moved facts: %+v, %v", got, err)
		}
		counts, err := repo.DirAudioCounts("/lib", []string{"work"})
		if err != nil || counts["work"] != 0 {
			t.Fatalf("extension counts: %v %v", counts, err)
		}
	}
	if err := repo.MoveInventoryPath(t.Context(), "/lib", "/lib/work/to/track.txt", ""); err != nil {
		t.Fatal(err)
	}
	if found, err := repo.RootExistsInInventory("/lib/work/to/track.txt"); err != nil || found {
		t.Fatalf("deleted file: %v %v", found, err)
	}
}
