package sqlite_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
)

// TestOpenRefusesOlderDatabase is the compatibility contract: a database that
// was not created by this schema generation is refused at open time. Nothing is
// converted, nothing is cleared, and the refusal names a new data directory
// instead of resetting someone's data (ADR 0013 §1).
func TestOpenRefusesOlderDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "older.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, execErr := db.Exec(`CREATE TABLE libraries (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		root_path TEXT NOT NULL DEFAULT ''
	)`); execErr != nil {
		t.Fatalf("create older schema: %v", execErr)
	}
	if _, execErr := db.Exec(
		"INSERT INTO libraries (id, name, root_path) VALUES ('lib-1', 'Music', '/music')",
	); execErr != nil {
		t.Fatalf("seed older library: %v", execErr)
	}
	if closeErr := db.Close(); closeErr != nil {
		t.Fatalf("close db: %v", closeErr)
	}

	repo, err := sqlite.NewRepository(dbPath)
	if repo != nil {
		repo.Close()
		t.Fatal("expected the older database to be refused")
	}
	if !errors.Is(err, sqlite.ErrIncompatibleDatabase) {
		t.Fatalf("expected sqlite.ErrIncompatibleDatabase, got %v", err)
	}
	if !strings.Contains(err.Error(), "ONSEI_DATA_DIR") {
		t.Errorf("refusal must name the way out, got: %v", err)
	}

	// The refused database is untouched: the row that was there is still there.
	check, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen for inspection: %v", err)
	}
	defer check.Close()
	var name string
	if scanErr := check.QueryRow("SELECT name FROM libraries WHERE id = 'lib-1'").Scan(&name); scanErr != nil {
		t.Fatalf("older row did not survive the refusal: %v", scanErr)
	}
	if name != "Music" {
		t.Errorf("older row changed: name = %q", name)
	}
}

// TestOpenRefusesAnotherSchemaVersion refuses a database whose marker names a
// different schema generation, even though the marker table exists.
func TestOpenRefusesAnotherSchemaVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "future.db")
	created, err := sqlite.NewRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, execErr := created.DB().
		Exec("INSERT INTO goose_db_version (version_id, is_applied) VALUES (99, 1)"); execErr != nil {
		t.Fatal(execErr)
	}
	if closeErr := created.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	repo, err := sqlite.NewRepository(dbPath)
	if repo != nil {
		repo.Close()
		t.Fatal("expected another schema version to be refused")
	}
	if !errors.Is(err, sqlite.ErrIncompatibleDatabase) {
		t.Fatalf("expected sqlite.ErrIncompatibleDatabase, got %v", err)
	}
	if !strings.Contains(err.Error(), "99") {
		t.Errorf("refusal must name the version it found, got: %v", err)
	}
}

// TestOpenCreatesAndReportsTheSchemaMarker opens a fresh database, writes the
// marker, and reopens it: a database created by this build is accepted by this
// build.
func TestOpenCreatesAndReportsTheSchemaMarker(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fresh.db")

	repo, err := sqlite.NewRepository(dbPath)
	if err != nil {
		t.Fatalf("sqlite.NewRepository on a fresh path failed: %v", err)
	}
	var marker string
	if scanErr := repo.DB().QueryRow(
		"SELECT value FROM schema_meta WHERE key = 'database_mode'",
	).Scan(&marker); scanErr != nil {
		t.Fatalf("read schema marker: %v", scanErr)
	}
	if marker != "release" {
		t.Errorf("marker = %q, want %q", marker, "release")
	}
	// The world outside the library domain is gone from a fresh database: the
	// overview reads the scanned inventory, and the derived folder table that
	// used to sit beside it no longer exists.
	var count int
	if scanErr := repo.DB().QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'library_folders'",
	).Scan(&count); scanErr != nil {
		t.Fatalf("inspect library_folders: %v", scanErr)
	}
	if count != 0 {
		t.Error("library_folders must not exist: the inventory is the only listing source")
	}
	if closeErr := repo.Close(); closeErr != nil {
		t.Fatalf("close repo: %v", closeErr)
	}

	reopened, err := sqlite.NewRepository(dbPath)
	if err != nil {
		t.Fatalf("reopen of a database this build created failed: %v", err)
	}
	reopened.Close()
}

// Existing development tables are user-controlled, not repaired on startup.
func TestOpenDoesNotRepairAnExistingDevelopmentDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "existing.db")
	repo, err := sqlite.NewDevelopmentRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, execErr := repo.DB().
		Exec("INSERT INTO libraries (id, name, root_path, root_path_key) VALUES ('lib-1', 'Music', '/music', '/music'); DROP TABLE common_settings;"); execErr != nil {
		t.Fatal(execErr)
	}
	if closeErr := repo.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	reopened, err := sqlite.NewDevelopmentRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var name string
	if queryErr := reopened.DB().
		QueryRow("SELECT name FROM libraries WHERE id = 'lib-1'").
		Scan(&name); queryErr != nil ||
		name != "Music" {
		t.Fatalf("existing row = %q, queryErr = %v", name, queryErr)
	}
	if _, checkErr := reopened.GetCommonSettings("conversion"); checkErr == nil {
		t.Fatal("startup must not recreate a manually removed table")
	}
}

func TestExistingCommonSettingsRowWithVersionColumnStillLoads(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "existing-common.db")
	repo, err := sqlite.NewRepository(dbPath)
	if err != nil {
		t.Fatalf("sqlite.NewRepository: %v", err)
	}
	if _, execErr := repo.DB().Exec(`
		ALTER TABLE common_settings ADD COLUMN schema_version INTEGER NOT NULL DEFAULT 0;
		INSERT INTO common_settings (operation_type, schema_version, settings_json, updated_at)
		VALUES ('conversion', 1, '{"mode":"strict"}', '2026-09-25T00:00:00Z');
	`); execErr != nil {
		t.Fatalf("seed existing common settings: %v", execErr)
	}
	if closeErr := repo.Close(); closeErr != nil {
		t.Fatalf("close repo: %v", closeErr)
	}

	reopened, err := sqlite.NewRepository(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	stored, err := reopened.GetCommonSettings("conversion")
	if err != nil || stored == nil || stored.Settings != `{"mode":"strict"}` {
		t.Fatalf("stored common settings = %+v, err = %v", stored, err)
	}
}
