package sqlite

import "io/fs"

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
