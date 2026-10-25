package sqlite //nolint:testpackage // white-box tests drive the real schema

import (
	"context"
	"testing"

	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/works"
)

func stageFile(t *testing.T, repo *Repository, session, root, parent, name string, size int64) {
	t.Helper()
	if err := stageTestRows(
		repo,
		inventory.StagingEntry{
			SessionID:  session,
			Path:       parent + "/" + name,
			RootPath:   root,
			ParentPath: parent,
			Name:       name,
			IsDir:      false,
			IsWorkRoot: false,
			Size:       size,
			Mtime:      7,
		},
	); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogChangedFiresOnceAfterEachCommitThatChangesTheListing(t *testing.T) {
	repo := newTestRepository(t)
	calls := 0
	repo.OnCatalogChanged = func() { calls++ }
	expect := func(want int, what string) {
		t.Helper()
		if calls != want {
			t.Fatalf("%s: %d notifications, want %d", what, calls, want)
		}
	}
	lib, err := repo.CreateLibrary("music", "/music")
	if err != nil {
		t.Fatal(err)
	}
	stageDir(t, repo, "scan", "/music", "RJ00000001 a")
	if _, err = repo.MergeStaging("scan", lib.ID, "/music"); err != nil {
		t.Fatal(err)
	}
	expect(1, "scan merge")

	if err = repo.SyncObservedInventory("/music", nil, []inventory.InventoryFile{
		{Path: "/music/RJ00000001 a/cover.jpg", Size: 3, Mtime: 5},
	}, nil); err != nil {
		t.Fatal(err)
	}
	expect(2, "observed inventory (cover publication)")

	if err = repo.RelocateDirectoryAs(
		context.Background(),
		"/music",
		"/music/RJ00000001 a",
		"/music/RJ00000001 b",
		"",
	); err != nil {
		t.Fatal(err)
	}
	expect(3, "relocation")
	// A failed relocation rolls back and says nothing.
	if err = repo.RelocateDirectoryAs(context.Background(), "/music", "/music/x", "/other/y", ""); err == nil {
		t.Fatal("invalid relocation succeeded")
	}
	expect(3, "rolled-back relocation")

	if _, err = repo.UpdateLibrary(lib.ID, new("renamed"), new("/music"), nil); err != nil {
		t.Fatal(err)
	}
	expect(3, "name-only update")
	if _, err = repo.UpdateLibrary(lib.ID, new("renamed"), new("/elsewhere"), nil); err != nil {
		t.Fatal(err)
	}
	expect(4, "root change")
	if err = repo.DeleteLibrary(lib.ID); err != nil {
		t.Fatal(err)
	}
	expect(5, "library delete")
}

func TestLocalCoversPicksOneStableCoverPerWorkFromTheInventory(t *testing.T) {
	repo := newTestRepository(t)
	lib, err := repo.CreateLibrary("music", "/music")
	if err != nil {
		t.Fatal(err)
	}
	stageDir(t, repo, "scan", "/music", "RJ00000001 a")
	stageDir(t, repo, "scan", "/music", "RJ00000001 b")
	stageDir(t, repo, "scan", "/music", "RJ00000002 c")
	stageFile(t, repo, "scan", "/music", "/music/RJ00000001 b", "cover.png", 11)
	stageFile(t, repo, "scan", "/music", "/music/RJ00000001 b", "track.mp3", 99)
	if _, err = repo.MergeStaging("scan", lib.ID, "/music"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ids := []works.WorkCode{"RJ00000001", "RJ00000002"}
	got, err := repo.LocalCovers(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	cover, ok := got["RJ00000001"]
	if !ok || cover.RelPath != "cover.png" || cover.Version == "" {
		t.Fatalf("cover = %+v, %v", cover, ok)
	}
	if _, has := got["RJ00000002"]; has {
		t.Fatal("a work without a cover file was given one")
	}

	// An earlier copy (by rel_path) wins once it has a cover, with a stable name order.
	if err = repo.SyncObservedInventory("/music", nil, []inventory.InventoryFile{
		{Path: "/music/RJ00000001 a/cover.webp", Size: 4, Mtime: 8},
		{Path: "/music/RJ00000001 a/cover.jpg", Size: 4, Mtime: 8},
	}, nil); err != nil {
		t.Fatal(err)
	}
	again, err := repo.LocalCovers(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	winner := again["RJ00000001"]
	if winner.CopyID == cover.CopyID || winner.RelPath != "cover.jpg" {
		t.Fatalf("winner = %+v, first copy was %+v", winner, cover)
	}

	// Observing new bytes at the path changes the version.
	if err = repo.SyncObservedInventory("/music", nil, []inventory.InventoryFile{
		{Path: "/music/RJ00000001 a/cover.jpg", Size: 5, Mtime: 9},
	}, nil); err != nil {
		t.Fatal(err)
	}
	third, _ := repo.LocalCovers(ctx, ids)
	if third["RJ00000001"].Version == winner.Version {
		t.Fatal("version did not change with the observed file")
	}
}
