package sqlite_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
)

func TestDevelopmentAndReleaseSchemasAgree(t *testing.T) {
	var schemas [][]string
	for mode, open := range map[string]func(string) (*sqlite.Repository, error){
		"development": sqlite.NewDevelopmentRepository,
		"release":     sqlite.NewRepository,
	} {
		repo, err := open(filepath.Join(t.TempDir(), "cache.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { repo.Close() })
		schemas = append(schemas, schemaSnapshot(t, repo.DB()))
		t.Run(mode, func(t *testing.T) {
			var slots int
			if err := repo.DB().
				QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='policy_slots'").
				Scan(&slots); err != nil || slots != 0 {
				t.Fatalf("removed policy table count = %d, err = %v", slots, err)
			}
			if _, execErr := repo.DB().
				Exec("INSERT INTO libraries (id, name, root_path, root_path_key) VALUES ('lib', 'Music', '/music', '/music'); INSERT INTO common_settings (operation_type, settings_json) VALUES ('conversion', '{}'); INSERT INTO works (id, collected_at, updated_at) VALUES ('RJ00000001', '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z');"); execErr != nil {
				t.Fatal(execErr)
			}
			settings, err := repo.GetCommonSettings("conversion")
			if err != nil || settings == nil || settings.Settings != "{}" {
				t.Fatalf("common settings = %+v, err = %v", settings, err)
			}
			if _, execErr := repo.DB().
				Exec("INSERT INTO work_user_data (work_id, rating) VALUES ('RJ00000001', 1.5)"); execErr == nil {
				t.Fatal("fractional rating must be refused")
			}
			if _, execErr := repo.DB().
				Exec("INSERT INTO work_user_data (work_id, rating) VALUES ('RJ00000001', 7)"); execErr != nil {
				t.Fatal(execErr)
			}
			if _, execErr := repo.DB().
				Exec("INSERT INTO work_copies (id, work_id, library_id, rel_path, created_at) VALUES ('copy', 'RJ00000001', 'missing', 'a', 'now')"); execErr == nil {
				t.Fatal("missing library foreign key must be refused")
			}
		})
	}
	if !reflect.DeepEqual(schemas[0], schemas[1]) {
		t.Fatalf("development and release schema differ:\n%v\n%v", schemas[0], schemas[1])
	}
}

func TestUnusedCompatibilitySchemaIsAbsent(t *testing.T) {
	for mode, open := range map[string]func(string) (*sqlite.Repository, error){
		"development": sqlite.NewDevelopmentRepository,
		"release":     sqlite.NewRepository,
	} {
		t.Run(mode, func(t *testing.T) {
			repo, err := open(filepath.Join(t.TempDir(), "cache.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repo.Close() })
			for table, columns := range map[string][]string{
				"entries":         {"path_posix", "file_size", "duration_ms"},
				"entries_staging": {"file_size", "duration_ms"},
				"works":           {"created_at", "thumbnail_url"},
			} {
				for _, column := range columns {
					var count int
					err := repo.DB().QueryRow(
						"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?", table, column,
					).Scan(&count)
					if err != nil {
						t.Fatal(err)
					}
					if count != 0 {
						t.Errorf("%s.%s still exists", table, column)
					}
				}
			}
			for _, index := range []string{"idx_entries_path_posix", "idx_classifier_tag_norm"} {
				var count int
				err := repo.DB().QueryRow(
					"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", index,
				).Scan(&count)
				if err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Errorf("%s still exists", index)
				}
			}
		})
	}
}

func schemaSnapshot(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(
		"SELECT type || ':' || name || ':' || sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%' AND name <> 'goose_db_version' ORDER BY type, name",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var schema []string
	for rows.Next() {
		var ddl string
		if checkErr := rows.Scan(&ddl); checkErr != nil {
			t.Fatal(checkErr)
		}
		// ALTER TABLE changes punctuation spacing without changing the schema.
		normalized := strings.Join(strings.Fields(ddl), " ")
		normalized = strings.NewReplacer(" ,", ",", " )", ")", "( ", "(").Replace(normalized)
		schema = append(schema, normalized)
	}
	if checkErr := rows.Err(); checkErr != nil {
		t.Fatal(checkErr)
	}
	return schema
}

func TestDatabaseModesStaySeparate(t *testing.T) {
	for mode, open := range map[string]func(string) (*sqlite.Repository, error){
		"development": sqlite.NewDevelopmentRepository,
		"release":     sqlite.NewRepository,
	} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cache.db")
			repo, err := open(path)
			if err != nil {
				t.Fatal(err)
			}
			if closeErr := repo.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			other := sqlite.NewRepository
			if mode == "release" {
				other = sqlite.NewDevelopmentRepository
			}
			if repo, err := other(path); !errors.Is(err, sqlite.ErrIncompatibleDatabase) {
				if repo != nil {
					repo.Close()
				}
				t.Fatalf("wrong mode must be refused: %v", err)
			}
			if mode == "release" {
				if _, err := sqlite.RebuildDevelopmentDatabase(path); !errors.Is(err, sqlite.ErrIncompatibleDatabase) {
					t.Fatalf("release rebuild must be refused: %v", err)
				}
			}
		})
	}
}

func TestDevelopmentRebuildPreservesBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	repo, err := sqlite.NewDevelopmentRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, execErr := repo.DB().
		Exec("INSERT INTO libraries (id, name, root_path, root_path_key) VALUES ('lib', 'Music', '/music', '/music'); DROP TABLE common_settings;"); execErr != nil {
		t.Fatal(execErr)
	}
	if closeErr := repo.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	backup, err := sqlite.RebuildDevelopmentDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := sqlite.NewDevelopmentRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer rebuilt.Close()
	if _, execErr := rebuilt.DB().
		Exec("INSERT INTO works(id,updated_at) VALUES('RJ00000002','now')"); execErr == nil {
		t.Fatal("rebuilt database accepted a work without collection time")
	}
	settings, err := rebuilt.GetCommonSettings("conversion")
	if err != nil || settings != nil {
		t.Fatalf("rebuilt common settings = %+v, err = %v", settings, err)
	}
	var count int
	if queryErr := rebuilt.DB().QueryRow("SELECT COUNT(*) FROM libraries").Scan(&count); queryErr != nil || count != 0 {
		t.Fatalf("rebuilt libraries = %d, queryErr = %v", count, queryErr)
	}
	check, err := sql.Open("sqlite", backup)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var name string
	if queryErr := check.QueryRow("SELECT name FROM libraries WHERE id = 'lib'").
		Scan(&name); queryErr != nil ||
		name != "Music" {
		t.Fatalf("backup library = %q, queryErr = %v", name, queryErr)
	}
	if _, execErr := check.Exec("SELECT * FROM common_settings"); execErr == nil {
		t.Fatal("backup must preserve manual schema changes")
	}
}

func TestDevelopmentRebuildFailurePreservesOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")
	repo, err := sqlite.NewDevelopmentRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, execErr := repo.DB().
		Exec("INSERT INTO libraries (id, name, root_path, root_path_key) VALUES ('lib', 'Music', '/music', '/music')"); execErr != nil {
		t.Fatal(execErr)
	}
	if closeErr := repo.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(dir, "backups"), []byte("blocked"), 0600); writeErr != nil {
		t.Fatal(writeErr)
	}
	if _, checkErr := sqlite.RebuildDevelopmentDatabase(path); checkErr == nil {
		t.Fatal("backup failure must stop rebuild")
	}
	reopened, err := sqlite.NewDevelopmentRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var name string
	if queryErr := reopened.DB().
		QueryRow("SELECT name FROM libraries WHERE id = 'lib'").
		Scan(&name); queryErr != nil ||
		name != "Music" {
		t.Fatalf("original library = %q, queryErr = %v", name, queryErr)
	}
}
