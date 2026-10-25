package sqlite //nolint:testpackage // white-box tests exercise unexported internals

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/Clementine33/preen/backend/internal/inventory"
)

func newTestRepository(t *testing.T) *Repository {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "onsei-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(tmpFile.Name()) })
	tmpFile.Close()

	repo, err := NewRepository(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	return repo
}

// stageTestRows builds observations through the same public staging seam as scans.
func stageTestRows(repo *Repository, rows ...inventory.StagingEntry) error {
	for _, row := range rows {
		if err := NewScanStaging(repo).WriteStagingEntries(row.SessionID, []inventory.StagingEntry{row}); err != nil {
			return err
		}
	}
	return nil
}

// stagingTestRow inspects the existing storage seam for atomicity regressions.
// Finished sessions have no handle, so their staging count is zero.
func stagingTestRow(repo *Repository, id, query string, args ...any) *sql.Row {
	repo.stagingMu.Lock()
	session := repo.staging[id]
	repo.stagingMu.Unlock()
	if session == nil {
		return repo.DB().QueryRow("SELECT 0")
	}
	return session.conn.QueryRowContext(context.Background(), query, args...)
}
