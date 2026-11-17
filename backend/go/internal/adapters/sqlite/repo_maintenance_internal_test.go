// The raw database below is built the way an earlier build did, which needs the
// unexported schema initializer.

package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// A database created before incremental auto-vacuum existed keeps the mode it
// was created with. This also pins the startup path for such a file: opening it
// through NewRepository, whose DSN now asks for incremental auto-vacuum, has to
// keep working rather than fail the whole process.
func TestRepository_AutoVacuumMode_ExistingDatabaseKeepsItsMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.db")

	// Build the file the way an earlier build did: the same schema, no pragmas.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	if err = initSchema(raw); err != nil {
		t.Fatalf("initSchema: %v", err)
	}
	if err = raw.Close(); err != nil {
		t.Fatalf("close raw database: %v", err)
	}

	repo, err := NewDevelopmentRepository(path)
	if err != nil {
		t.Fatalf("NewRepository on an existing database: %v", err)
	}
	defer repo.Close()

	mode, err := repo.AutoVacuumMode(t.Context())
	if err != nil {
		t.Fatalf("AutoVacuumMode: %v", err)
	}
	if mode != AutoVacuumNone {
		t.Errorf(
			"AutoVacuumMode = %d, want %d: an existing file keeps the mode it was created with",
			mode,
			AutoVacuumNone,
		)
	}
}
