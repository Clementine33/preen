package sqlite_test

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
)

func releaseFixture(t *testing.T, changes ...string) fstest.MapFS {
	t.Helper()
	initial := `-- +goose Up
CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO schema_meta (key,value) VALUES ('database_mode','release');
CREATE TABLE libraries (id TEXT PRIMARY KEY, name TEXT NOT NULL, root_path TEXT NOT NULL, root_path_key TEXT NOT NULL);
`
	files := fstest.MapFS{"00001_initial.sql": {Data: []byte(initial)}}
	names := []string{"00002_second.sql", "00003_third.sql"}
	for i, change := range changes {
		files[names[i]] = &fstest.MapFile{Data: []byte("-- +goose Up\n" + change)}
	}
	return files
}

func migrationVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var version int
	if queryErr := db.QueryRow("SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1").
		Scan(&version); queryErr != nil {
		t.Fatal(queryErr)
	}
	return version
}

func TestReleaseUpgradePreservesWALDataAndBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	initial, err := sqlite.OpenReleaseWithMigrations(path, releaseFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { initial.Close() })
	// Keep the first connection open to prevent its WAL being checkpointed on close.
	if _, execErr := initial.DB().
		Exec("PRAGMA wal_autocheckpoint = 0; INSERT INTO libraries (id, name, root_path, root_path_key) VALUES ('lib', 'Music', '/music', '/music');"); execErr != nil {
		t.Fatal(execErr)
	}
	wal, err := os.Stat(path + "-wal")
	if err != nil || wal.Size() == 0 {
		t.Fatalf("expected committed WAL content: %v", err)
	}
	for _, count := range []int{1, 2} {
		t.Run([]string{"previous release", "skip a release"}[count-1], func(t *testing.T) {
			changes := []string{
				"ALTER TABLE libraries ADD COLUMN migration_note TEXT NOT NULL DEFAULT ''; UPDATE libraries SET migration_note = 'kept';",
				"CREATE TABLE release_extra (id INTEGER PRIMARY KEY);",
			}
			upgradePath := path
			if count == 2 {
				upgradePath = filepath.Join(filepath.Dir(path), "skip.db")
				old, oldErr := sqlite.OpenReleaseWithMigrations(upgradePath, releaseFixture(t))
				if oldErr != nil {
					t.Fatal(oldErr)
				}
				defer old.Close()
				if _, execErr := old.DB().
					Exec("INSERT INTO libraries (id, name, root_path, root_path_key) VALUES ('lib', 'Music', '/music', '/music')"); execErr != nil {
					t.Fatal(execErr)
				}
			}
			upgraded, upgradeErr := sqlite.OpenReleaseWithMigrations(upgradePath, releaseFixture(t, changes[:count]...))
			if upgradeErr != nil {
				t.Fatal(upgradeErr)
			}
			defer upgraded.Close()
			var name, note string
			if queryErr := upgraded.DB().
				QueryRow("SELECT name, migration_note FROM libraries WHERE id = 'lib'").
				Scan(&name, &note); queryErr != nil || name != "Music" ||
				note != "kept" {
				t.Fatalf("upgraded data = %q, %q, queryErr = %v", name, note, queryErr)
			}
			if version := migrationVersion(t, upgraded.DB()); version != count+1 {
				t.Fatalf("migration version = %d", version)
			}
		})
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), "backups", "*.db"))
	if err != nil || len(backups) != 2 {
		t.Fatalf("backups = %v, err = %v", backups, err)
	}
	assertOriginalBackupRestorable(t, backups)
}

func assertOriginalBackupRestorable(t *testing.T, backups []string) {
	t.Helper()
	// Restore the pre-upgrade snapshot using the binary that produced it.
	for _, backup := range backups {
		db, err := sql.Open("sqlite", backup)
		if err != nil {
			t.Fatal(err)
		}
		version := migrationVersion(t, db)
		if closeErr := db.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if version != 1 {
			continue
		}
		contents, err := os.ReadFile(backup)
		if err != nil {
			t.Fatal(err)
		}
		restoredPath := filepath.Join(t.TempDir(), "restored.db")
		if writeErr := os.WriteFile(restoredPath, contents, 0600); writeErr != nil {
			t.Fatal(writeErr)
		}
		restored, err := sqlite.OpenReleaseWithMigrations(restoredPath, releaseFixture(t))
		if err != nil {
			t.Fatal(err)
		}
		defer restored.Close()
		var name string
		if queryErr := restored.DB().
			QueryRow("SELECT name FROM libraries WHERE id = 'lib'").
			Scan(&name); queryErr != nil ||
			name != "Music" {
			t.Fatalf("restored WAL data = %q, queryErr = %v", name, queryErr)
		}
		return
	}
	t.Fatal("no pre-upgrade backup survived")
}

func TestReleaseMigrationFailureCanBeRetried(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	initial, err := sqlite.OpenReleaseWithMigrations(path, releaseFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := initial.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	second := "CREATE TABLE migration_probe (value TEXT); INSERT INTO migration_probe VALUES ('preserved');"
	broken := "CREATE TABLE failed_probe (value TEXT); INSERT INTO missing_table VALUES ('bad');"
	if repo, failureErr := sqlite.OpenReleaseWithMigrations(
		path,
		releaseFixture(t, second, broken),
	); failureErr == nil {
		repo.Close()
		t.Fatal("broken migration must fail startup")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if version := migrationVersion(t, db); version != 2 {
		t.Fatalf("successful earlier migration did not survive: version %d", version)
	}
	var count int
	if queryErr := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name = 'failed_probe'").
		Scan(&count); queryErr != nil ||
		count != 0 {
		t.Fatalf("failed migration was not rolled back: count %d, queryErr %v", count, queryErr)
	}
	if closeErr := db.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	fixed := "ALTER TABLE migration_probe ADD COLUMN repaired INTEGER NOT NULL DEFAULT 1;"
	retried, err := sqlite.OpenReleaseWithMigrations(path, releaseFixture(t, second, fixed))
	if err != nil {
		t.Fatal(err)
	}
	defer retried.Close()
	var value string
	if queryErr := retried.DB().
		QueryRow("SELECT value FROM migration_probe WHERE repaired = 1").
		Scan(&value); queryErr != nil ||
		value != "preserved" {
		t.Fatalf("retry data = %q, queryErr = %v", value, queryErr)
	}
	if version := migrationVersion(t, retried.DB()); version != 3 {
		t.Fatalf("retry version = %d", version)
	}
}

func TestBackupFailureStopsMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")
	repo, err := sqlite.OpenReleaseWithMigrations(path, releaseFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := repo.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(dir, "backups"), []byte("blocked"), 0600); writeErr != nil {
		t.Fatal(writeErr)
	}
	if repo, migrationErr := sqlite.OpenReleaseWithMigrations(
		path,
		releaseFixture(t, "CREATE TABLE should_not_exist (id INTEGER);"),
	); migrationErr == nil {
		repo.Close()
		t.Fatal("backup failure must stop startup")
	} else if !strings.Contains(
		migrationErr.Error(),
		"backup",
	) {
		t.Fatalf("expected backup error, got %v", migrationErr)
	}
	reopened, err := sqlite.OpenReleaseWithMigrations(path, releaseFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if version := migrationVersion(t, reopened.DB()); version != 1 {
		t.Fatalf("backup failure advanced migration to %d", version)
	}
}

func TestFailedInitialMigrationCanBeRetried(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	broken := fstest.MapFS{
		"00001_initial.sql": {
			Data: []byte("-- +goose Up\nCREATE TABLE partial (id INTEGER); INSERT INTO missing_table VALUES (1);"),
		},
	}
	if repo, err := sqlite.OpenReleaseWithMigrations(path, broken); err == nil {
		repo.Close()
		t.Fatal("initial migration must fail")
	}
	repo, err := sqlite.NewRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if version := migrationVersion(t, repo.DB()); version != 8 {
		t.Fatalf("initial retry version = %d", version)
	}
}

func TestOldUnreleasedVersionsAreBackedUpAndRefused(t *testing.T) {
	for version := 1; version <= 7; version++ {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cache.db")
			files := releaseFixture(t)
			for n := 2; n <= version; n++ {
				files[fmt.Sprintf("%05d_legacy.sql", n)] = &fstest.MapFile{
					Data: []byte("-- +goose Up\nINSERT INTO legacy_probe VALUES ('later');"),
				}
			}
			files["00001_initial.sql"] = &fstest.MapFile{Data: []byte(`-- +goose Up
CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO schema_meta (key,value) VALUES ('database_mode','release');
CREATE TABLE entries (id TEXT PRIMARY KEY);
CREATE TABLE legacy_probe (value TEXT NOT NULL);
INSERT INTO legacy_probe VALUES ('original');`)}
			old, err := sqlite.OpenReleaseWithMigrations(path, files)
			if err != nil {
				t.Fatal(err)
			}
			if closeErr := old.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			repo, err := sqlite.NewRepository(path)
			if repo != nil {
				repo.Close()
				t.Fatal("old version opened")
			}
			if err == nil || !strings.Contains(err.Error(), "table schema_meta already exists") {
				t.Fatalf("expected initial table collision: %v", err)
			}
			backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), "backups", "*.db"))
			if err != nil || len(backups) != 1 {
				t.Fatalf("backups = %v, err = %v", backups, err)
			}
			for _, file := range []string{path, backups[0]} {
				db, err := sql.Open("sqlite", file)
				if err != nil {
					t.Fatal(err)
				}
				if got := migrationVersion(t, db); got != version {
					t.Fatalf("%s version = %d", file, got)
				}
				var count int
				queryErr := db.QueryRow("SELECT COUNT(*) FROM legacy_probe").Scan(&count)
				if queryErr != nil || count != version {
					t.Fatalf("%s probe count = %d, %v", file, count, queryErr)
				}
				db.Close()
			}
		})
	}
}
