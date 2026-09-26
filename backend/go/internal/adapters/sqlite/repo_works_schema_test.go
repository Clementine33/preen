package sqlite //nolint:testpackage // white-box tests read the schema the repository created

import (
	"strings"
	"testing"
)

// TestCatalogTablesAreStrict: every table of the catalog is a STRICT table, so
// an INTEGER column cannot quietly store a real. Type affinity alone would let
// 1.5 into a rating column and leave only a range CHECK to catch it - and the
// range CHECK itself cannot catch it, because 1.5 is inside 1..10.
func TestCatalogTablesAreStrict(t *testing.T) {
	repo := newTestRepository(t)
	for _, table := range []string{
		"works", "circles", "voice_actors", "work_voice_actors", "tags", "work_tags",
		"work_statistics", "work_source_state", "work_relation_self", "work_declared_children",
		"work_translations", "work_language_editions", "work_copies", "work_user_data",
		"user_tags", "work_user_tags",
	} {
		var ddl string
		if err := repo.DB().QueryRow(
			"SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", table,
		).Scan(&ddl); err != nil {
			t.Fatalf("table %s is missing: %v", table, err)
		}
		if !strings.Contains(ddl, ") STRICT") {
			t.Errorf("table %s is not STRICT:\n%s", table, ddl)
		}
	}
}

// TestCatalogColumnConstraints walks the values that must be refused and the
// ones that must be accepted. Each refusal is asserted with the statement's own
// error, so a constraint that merely moved is still caught.
func TestCatalogColumnConstraints(t *testing.T) {
	repo := newTestRepository(t)
	work := seedWork(t, repo, "RJ00000001", true)

	refused := []struct {
		name string
		stmt string
	}{
		{"a fractional rating", "INSERT INTO work_user_data (work_id, rating) VALUES ('RJ00000001', 1.5)"},
		{"a zero rating", "INSERT INTO work_user_data (work_id, rating) VALUES ('RJ00000001', 0)"},
		{"a rating above ten", "INSERT INTO work_user_data (work_id, rating) VALUES ('RJ00000001', 11)"},
		{"a negative sale count", "INSERT INTO work_statistics (work_id, dl_count) VALUES ('RJ00000001', -1)"},
		{"a fractional sale count", "INSERT INTO work_statistics (work_id, dl_count) VALUES ('RJ00000001', 1.5)"},
		{"an unknown adult flag value", "UPDATE works SET nsfw = 2 WHERE id = 'RJ00000001'"},
		{"a slashed release date", "UPDATE works SET release_date = '2025/06/25' WHERE id = 'RJ00000001'"},
		{"an unpadded release date", "UPDATE works SET release_date = '2025-6-5' WHERE id = 'RJ00000001'"},
		{
			"sales attributed to asmr.one",
			"INSERT INTO work_source_state (work_id, data_group, source, fetched_at) VALUES ('RJ00000001', 'statistics', 'asmr_one', '" + testStamp + "')",
		},
		{
			"an unknown data group",
			"INSERT INTO work_source_state (work_id, data_group, source, fetched_at) VALUES ('RJ00000001', 'everything', 'dlsite', '" + testStamp + "')",
		},
		{
			"an unknown relation role",
			"INSERT INTO work_relation_self (work_id, role, source, fetched_at) VALUES ('RJ00000001', 'sequel', 'dlsite', '" + testStamp + "')",
		},
	}
	for _, tc := range refused {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			if _, err := repo.DB().Exec(tc.stmt); err == nil {
				t.Fatalf("%q was accepted", tc.stmt)
			}
			// Undo whatever the statement managed to write before it failed.
			t.Cleanup(func() {
				_, _ = repo.DB().Exec("DELETE FROM work_user_data WHERE work_id = 'RJ00000001'")
				_, _ = repo.DB().Exec("DELETE FROM work_statistics WHERE work_id = 'RJ00000001'")
				_, _ = repo.DB().Exec("DELETE FROM work_source_state WHERE work_id = 'RJ00000001'")
				_, _ = repo.DB().Exec("DELETE FROM work_relation_self WHERE work_id = 'RJ00000001'")
				_, _ = repo.DB().Exec("UPDATE works SET nsfw = NULL, release_date = NULL WHERE id = 'RJ00000001'")
			})
		})
	}

	accepted := []struct {
		name string
		stmt string
	}{
		{"a rating inside the range", "UPDATE work_user_data SET rating = 7 WHERE work_id = 'RJ00000001'"},
		{"a cleared rating", "UPDATE work_user_data SET rating = NULL WHERE work_id = 'RJ00000001'"},
		{"no sale count", "UPDATE work_statistics SET dl_count = NULL WHERE work_id = 'RJ00000001'"},
		{"zero sales", "UPDATE work_statistics SET dl_count = 0 WHERE work_id = 'RJ00000001'"},
		{"a padded release date", "UPDATE works SET release_date = '2025-06-25' WHERE id = 'RJ00000001'"},
		{"a boolean adult flag", "UPDATE works SET nsfw = 1 WHERE id = 'RJ00000001'"},
		{"an unknown adult flag", "UPDATE works SET nsfw = NULL WHERE id = 'RJ00000001'"},
		{
			"basic data from asmr.one",
			"INSERT OR REPLACE INTO work_source_state (work_id, data_group, source, fetched_at) VALUES ('RJ00000001', 'core', 'asmr_one', '" + testStamp + "')",
		},
	}
	for _, tc := range accepted {
		if _, err := repo.DB().Exec(tc.stmt); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}

	// The child edge needs the declared child to exist, which is what makes a
	// declared child always reachable.
	if _, err := repo.DB().Exec(
		"INSERT INTO work_declared_children (declaring_work_id, child_work_id, source, fetched_at) VALUES ('RJ00000001', 'RJ00000009', 'dlsite', '" + testStamp + "')",
	); err == nil {
		t.Error("a declaration pointing at a work that does not exist was accepted")
	}
	_ = work
}

// TestOneEditionPerLanguage: the edition list is a set of language editions, so
// a second entry for the same language is a storage refusal rather than a
// duplicate row.
func TestOneEditionPerLanguage(t *testing.T) {
	repo := newTestRepository(t)
	seedWork(t, repo, "RJ00000001", true)
	insert := func(language string) error {
		_, err := repo.DB().Exec(`
			INSERT INTO work_language_editions
			    (work_id, version_root_rj, language, display_order)
			VALUES ('RJ00000001', 'RJ00000002', ?, 1)
		`, language)
		return err
	}
	if err := insert("JPN"); err != nil {
		t.Fatalf("the first edition was refused: %v", err)
	}
	if err := insert("JPN"); err == nil {
		t.Error("a second edition for the same language was accepted")
	}
	if err := insert("CHI_HANS"); err != nil {
		t.Errorf("a second language was refused: %v", err)
	}
}
