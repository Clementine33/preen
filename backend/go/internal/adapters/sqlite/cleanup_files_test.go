package sqlite_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/cleanup"
)

func TestCleanupCatalogNeverRemovesMediaOrDerivedFiles(t *testing.T) {
	repo := deletionRepository(t)
	root := t.TempDir()
	names := []string{"audio.flac", "cover.jpg", "thumbnail.cache", "recovery.flac"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	deletionSQL(
		t,
		repo,
		`UPDATE works SET source_url=?,cover_url=? WHERE id='RJ00000001'`,
		root,
		filepath.Join(root, "cover.jpg"),
	)
	deletionSQL(t, repo, `INSERT INTO tags(id,source,name) VALUES('unused','dlsite','unused')`)
	startCatalogRun(t, repo)
	for _, category := range []cleanup.Category{cleanup.Works, cleanup.SourceTags} {
		ids, err := repo.CleanupCandidates(t.Context(), category, time.Now(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if batchErr := repo.CleanupBatch(t.Context(), "catalog", category, time.Now(), ids); batchErr != nil {
			t.Fatal(batchErr)
		}
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != name {
			t.Fatalf("file %s changed: %q %v", name, data, err)
		}
	}
}
