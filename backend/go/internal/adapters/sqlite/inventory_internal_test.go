package sqlite

import (
	"testing"

	"github.com/Clementine33/preen/backend/internal/inventory"
)

func inventoryEntry(t testing.TB, repo *Repository, path string) inventory.Entry {
	t.Helper()
	entries, err := repo.ListEntriesUnderPath(path)
	if err != nil || len(entries) == 0 || entries[0].Path != path {
		t.Fatalf("entry %s: entries=%+v err=%v", path, entries, err)
	}
	return entries[0]
}
