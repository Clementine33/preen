package sqlite_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

// defaultBatchSize mirrors the production staging and merge batch size, so tests
// can build inputs that cross exactly one batch boundary.
const defaultBatchSize = 1000

// splitTestPath and joinTestPath build the parent/name columns of staged rows
// for the POSIX-style paths these tests use.
func splitTestPath(p string) (dir, name string) {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return ".", p
	}
	if i == 0 {
		return "/", p[1:]
	}
	return p[:i], p[i+1:]
}

func joinTestPath(dir, name string) string {
	return strings.TrimSuffix(dir, "/") + "/" + name
}

func newTestRepository(t *testing.T) *sqlite.Repository {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "onsei-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(tmpFile.Name()) })
	tmpFile.Close()

	repo, err := sqlite.NewRepository(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	return repo
}

// stageTestRows builds observations through the same public staging seam as scans.
func stageTestRows(repo *sqlite.Repository, rows ...inventory.StagingEntry) error {
	for _, row := range rows {
		if err := sqlite.NewScanStaging(repo).
			WriteStagingEntries(row.SessionID, []inventory.StagingEntry{row}); err != nil {
			return err
		}
	}
	return nil
}
