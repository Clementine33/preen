package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io/fs"
	"time"
)

// OpenReleaseWithMigrations exposes only the migration source seam, so tests
// can model multiple published binaries against the public repository behavior.
func OpenReleaseWithMigrations(dbPath string, migrations fs.FS) (*Repository, error) {
	return openRepository(dbPath, false, migrations)
}

// PauseAfterFirstStagingChunk exposes a driver checkpoint after the first root
// batch commits, without replacing the public persistence workflow.
func PauseAfterFirstStagingChunk(repo *Repository, checkpoint func()) error {
	return pauseAfterFirstStagingChunk(repo, checkpoint)
}

// SetAfterMergeBatch runs checkpoint after each selected-merge batch commits, so
// tests can interrupt a merge between batches.
func SetAfterMergeBatch(repo *Repository, checkpoint func()) { repo.afterMergeBatch = checkpoint }

// SetImportBeginWait records, per ImportMetadata call, the Go queue time and
// the db.Begin time, so measurements can report the combined wait.
func SetImportBeginWait(repo *Repository, record func(queue, begin time.Duration)) {
	repo.importBeginWait = record
}

// WrapRepositoryDriver reopens the repository's connection pool through wrap, so
// tests can observe every statement without replacing the public workflows.
func WrapRepositoryDriver(repo *Repository, wrap func(driver.Driver) driver.Driver) error {
	original := repo.db
	repo.db = sql.OpenDB(repositoryConnector{driver: wrap(original.Driver()), dsn: repo.dbPath + connectionPragmas})
	return original.Close()
}

// QueryStagingRow runs query on the session's private staging connection. A
// finished session has no handle, so its query reads as a single zero row.
func QueryStagingRow(repo *Repository, sessionID, query string, args ...any) *sql.Row {
	repo.stagingMu.Lock()
	session := repo.staging[sessionID]
	repo.stagingMu.Unlock()
	if session == nil {
		return repo.DB().QueryRow("SELECT 0")
	}
	return session.conn.QueryRowContext(context.Background(), query, args...)
}

// ImportLockHeld reports whether an ImportMetadata call currently holds the
// repository's import lock.
func ImportLockHeld(repo *Repository) bool {
	if repo.importMu.TryLock() {
		repo.importMu.Unlock()
		return false
	}
	return true
}

// SetScanStagingExecSeam makes the staging write fail with fn's error after the
// given number of exec calls, so tests can interrupt a batch midway.
func SetScanStagingExecSeam(staging *ScanStaging, fn func(callCount int) error) {
	staging.testExecSeam = fn
}
