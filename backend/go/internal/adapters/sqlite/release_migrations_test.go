package sqlite_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/library"
	"github.com/onsei/organizer/backend/internal/works"
)

func releaseFixture(t *testing.T, changes ...string) fstest.MapFS {
	t.Helper()
	initial, err := os.ReadFile("migrations/00001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{"00001_initial.sql": {Data: initial}}
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
	if version := migrationVersion(t, repo.DB()); version != 7 {
		t.Fatalf("initial retry version = %d", version)
	}
}

func TestMainCoverMigrationPreservesExistingWorkData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	initial, err := sqlite.OpenReleaseWithMigrations(path, releaseFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = initial.DB().
		Exec(`INSERT INTO works(id,created_at,updated_at) VALUES('RJ00000001','now','now');INSERT INTO work_user_data(work_id,rating,note) VALUES('RJ00000001',8,'keep')`); err != nil {
		t.Fatal(err)
	}
	if err = initial.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := sqlite.NewRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var note string
	var rating int
	if err = upgraded.DB().
		QueryRow(`SELECT rating,note FROM work_user_data WHERE work_id='RJ00000001'`).
		Scan(&rating, &note); err != nil || rating != 8 ||
		note != "keep" {
		t.Fatalf("data changed: %d %s %v", rating, note, err)
	}
	if _, err = upgraded.DB().Exec(`SELECT * FROM cover_candidates;SELECT * FROM cover_deliveries`); err != nil {
		t.Fatal(err)
	}
}

func TestAgeMigrationPreservesDataAndDoesNotGuessNonAdultAges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	old, err := sqlite.OpenReleaseWithMigrations(path, releaseFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = old.DB().Exec(`
		INSERT INTO works(id,nsfw,title,created_at,updated_at) VALUES
		('RJ00000001',1,'adult','now','now'),('RJ00000002',0,'non-adult','now','now'),('RJ00000003',NULL,'unreported','now','now');
		INSERT INTO work_user_data(work_id,rating,note) VALUES ('RJ00000002',8,'keep');
	`); err != nil {
		t.Fatal(err)
	}
	if err = old.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := sqlite.NewRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	for _, id := range []string{"RJ00000001", "RJ00000002", "RJ00000003"} {
		var age sql.NullInt64
		if err = upgraded.DB().QueryRow("SELECT age_category FROM works WHERE id=?", id).Scan(&age); err != nil {
			t.Fatal(err)
		}
		if id == "RJ00000001" && (!age.Valid || age.Int64 != 3) || id != "RJ00000001" && age.Valid {
			t.Fatalf("guessed or lost age for %s: %+v", id, age)
		}
	}
	var note, title string
	var rating int
	if err = upgraded.DB().
		QueryRow(`SELECT w.title,u.rating,u.note FROM works w JOIN work_user_data u ON w.id=u.work_id WHERE w.id='RJ00000002'`).
		Scan(&title, &rating, &note); err != nil || title != "non-adult" || rating != 8 ||
		note != "keep" {
		t.Fatalf("data lost during upgrade: %s %d %s %v", title, rating, note, err)
	}
}

func TestLanguageMigrationPreservesRelationsWithoutGuessingNM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	old, err := sqlite.OpenReleaseWithMigrations(path, releaseFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = old.DB().Exec(`
		INSERT INTO works(id,title,collected_at,created_at,updated_at) VALUES
		('RJ00000001','translated','now','now','now'),('RJ00000002','unknown','now','now','now');
		INSERT INTO work_translations(work_id,role,language) VALUES ('RJ00000001','child','CHI_HANT');
		INSERT INTO work_user_data(work_id,rating,note) VALUES ('RJ00000002',8,'keep');
	`); err != nil {
		t.Fatal(err)
	}
	if err = old.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := sqlite.NewRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	for _, id := range []works.WorkCode{"RJ00000001", "RJ00000002"} {
		detail, found, readErr := upgraded.GetWork(id)
		if readErr != nil || !found {
			t.Fatalf("read upgraded work: %v, %v", found, readErr)
		}
		if id == "RJ00000001" {
			if detail.Language == nil || *detail.Language != "CHI_HANT" {
				t.Fatal("lost legacy language")
			}
		} else if detail.Language != nil || detail.Note != "keep" || detail.Rating == nil || *detail.Rating != 8 {
			t.Fatalf("guessed language or lost user data: %+v", detail)
		}
	}
	if _, err = upgraded.DB().Exec("UPDATE works SET language='NM' WHERE id='RJ00000002'"); err != nil {
		t.Fatal(err)
	}
	detail, _, err := upgraded.GetWork("RJ00000002")
	if err != nil || detail.Language == nil || *detail.Language != "NM" {
		t.Fatalf("NM read/write: %+v, %v", detail, err)
	}
}

func TestLibraryStorageClassUpgradePreservesExistingLibrary(t *testing.T) {
	for _, from := range []string{"00001_initial.sql", "00006_work_collections.sql"} {
		t.Run(from, func(t *testing.T) {
			files := releaseMigrationsThrough(t, from)
			path := filepath.Join(t.TempDir(), "release.db")
			old, err := sqlite.OpenReleaseWithMigrations(path, files)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = old.DB().
				Exec(`INSERT INTO libraries(id,name,root_path,root_path_key) VALUES('existing','Music','/music','/music')`); err != nil {
				t.Fatal(err)
			}
			if err = old.Close(); err != nil {
				t.Fatal(err)
			}
			upgraded, err := sqlite.NewRepository(path)
			if err != nil {
				t.Fatal(err)
			}
			defer upgraded.Close()
			lib, err := upgraded.GetLibrary("existing")
			if err != nil {
				t.Fatal(err)
			}
			if lib.Name != "Music" || lib.RootPath != "/music" || lib.StorageClass != library.StorageHDD {
				t.Fatalf("upgraded library=%+v", lib)
			}
			class := library.StorageSSD
			lib, err = library.NewService(upgraded, nil, upgraded, nil).Patch(lib.ID, nil, nil, &class)
			if err != nil || lib.StorageClass != library.StorageSSD {
				t.Fatalf("upgraded storage edit=%+v, %v", lib, err)
			}
			fresh, err := upgraded.CreateLibrary("New", "/new")
			if err != nil || fresh.StorageClass != library.StorageHDD {
				t.Fatalf("fresh storage=%+v, %v", fresh, err)
			}
		})
	}
}

func releaseMigrationsThrough(t *testing.T, last string) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{}
	names, err := os.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name.Name() > last {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join("migrations", name.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		files[name.Name()] = &fstest.MapFile{Data: data}
	}
	return files
}
