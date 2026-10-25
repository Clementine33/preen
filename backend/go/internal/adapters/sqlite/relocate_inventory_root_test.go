package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"
)

func TestRelocateDirectoryRebasesStoredInventoryRoot(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/lib/source/音_%/track.opus", RootPath: "/lib/source"},
	)
	if err := repo.RelocateDirectory(t.Context(), "/lib", "/lib/source", "/lib/target"); err != nil {
		t.Fatal(err)
	}
	var root string
	if err := repo.DB().
		QueryRow(`SELECT root_path FROM entries WHERE path=?`, "/lib/target/音_%/track.opus").
		Scan(&root); err != nil {
		t.Fatal(err)
	}
	if root != "/lib/target" {
		t.Fatalf("root_path = %q, want /lib/target", root)
	}
}
