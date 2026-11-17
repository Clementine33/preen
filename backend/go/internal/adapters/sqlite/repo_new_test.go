package sqlite_test

import (
	"database/sql"
	"strings"
	"testing"
)

func TestNewRepository(t *testing.T) {
	t.Run("sets inventory primary keys", func(t *testing.T) {
		repo := newTestRepository(t)
		for _, key := range []struct {
			table, column string
			position      int
		}{
			{"inventory_dirs", "id", 1}, {"inventory_files", "dir_ref", 1}, {"inventory_files", "name", 2},
		} {
			var pk int
			err := repo.DB().
				QueryRow("SELECT pk FROM pragma_table_info(?) WHERE name=?", key.table, key.column).
				Scan(&pk)
			if err != nil || pk != key.position {
				t.Errorf("%s.%s primary key = %d, want %d: %v", key.table, key.column, pk, key.position, err)
			}
		}
		var schema string
		if err := repo.DB().QueryRow("SELECT sql FROM sqlite_master WHERE name='inventory_files'").
			Scan(&schema); err != nil ||
			!strings.Contains(schema, "WITHOUT ROWID") {
			t.Fatalf("files must use WITHOUT ROWID: %s, %v", schema, err)
		}
		var legacy int
		if err := repo.DB().QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='entries'").
			Scan(&legacy); err != nil ||
			legacy != 0 {
			t.Fatalf("legacy entries table/view remains: %d, %v", legacy, err)
		}
	})

	t.Run("enables WAL journal mode", func(t *testing.T) {
		repo := newTestRepository(t)

		var mode string
		err := repo.DB().QueryRow("PRAGMA journal_mode").Scan(&mode)
		if err != nil {
			t.Fatalf("failed to query journal mode: %v", err)
		}
		if !strings.EqualFold(mode, "wal") {
			t.Fatalf("expected WAL journal mode, got %q", mode)
		}
	})
}

func TestConnectionPragmasAreAppliedToEveryConnection(t *testing.T) {
	repo := newTestRepository(t)
	ctx := t.Context()

	// Two connections held at once: the pragmas are connection-scoped, so a
	// second one is exactly what a one-shot PRAGMA statement fails to configure.
	first, err := repo.DB().Conn(ctx)
	if err != nil {
		t.Fatalf("hold first connection: %v", err)
	}
	defer first.Close()
	second, err := repo.DB().Conn(ctx)
	if err != nil {
		t.Fatalf("hold second connection: %v", err)
	}
	defer second.Close()

	for name, conn := range map[string]*sql.Conn{"first": first, "second": second} {
		var foreignKeys, busyTimeout int
		if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
			t.Fatalf("%s connection: read foreign_keys: %v", name, err)
		}
		if foreignKeys != 1 {
			t.Errorf("%s connection: foreign_keys = %d, want 1", name, foreignKeys)
		}
		if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
			t.Fatalf("%s connection: read busy_timeout: %v", name, err)
		}
		if busyTimeout != 5000 {
			t.Errorf("%s connection: busy_timeout = %d, want 5000", name, busyTimeout)
		}
	}
}

func TestEntriesPathLookupIsIndexedWithoutADuplicateIndex(t *testing.T) {
	repo := newTestRepository(t)

	var count int
	if err := repo.DB().QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_entries_path'",
	).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 0 {
		t.Fatalf("idx_entries_path duplicates the primary key's own index, got %d", count)
	}

	rows, err := repo.DB().Query(
		"EXPLAIN QUERY PLAN SELECT path FROM inventory_dirs WHERE path = ? OR (path >= ? AND path < ?)",
		"/music/a", "/music/a/", "/music/a0",
	)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan = append(plan, detail)
	}
	if len(plan) == 0 {
		t.Fatal("no query plan")
	}
	// The index that serves this is whichever one SQLite built for the primary
	// key; what matters is that the lookup searches it instead of scanning.
	for _, step := range plan {
		if strings.HasPrefix(step, "SCAN") {
			t.Fatalf("path lookup scans the table: %v", plan)
		}
	}
}

func TestRepoSchemaInit(t *testing.T) {
	repo := newTestRepository(t)

	var count int
	err := repo.DB().QueryRow("SELECT COUNT(*) FROM inventory_dirs").Scan(&count)
	if err != nil {
		t.Fatalf("entries table not found: %v", err)
	}

	err = repo.DB().QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='entries_staging'").Scan(&count)
	if err != nil || count != 0 {
		t.Fatalf("main staging table exists: count=%d, err=%v", count, err)
	}
}
