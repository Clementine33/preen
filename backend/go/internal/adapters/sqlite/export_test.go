package sqlite

import "io/fs"

// OpenReleaseWithMigrations exposes only the migration source seam, so tests
// can model multiple published binaries against the public repository behavior.
func OpenReleaseWithMigrations(dbPath string, migrations fs.FS) (*Repository, error) {
	return openRepository(dbPath, false, migrations)
}
