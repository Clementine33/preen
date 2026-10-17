package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
)

func TestNewConnectionReadsWhileAnotherConnectionHoldsWriteTransaction(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	mode, err := repo.AutoVacuumMode(t.Context())
	if err != nil || mode != sqlite.AutoVacuumIncremental {
		t.Fatalf("new file auto_vacuum = %d, %v", mode, err)
	}
	db := repo.DB()
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(0)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE schema_meta SET value = value"); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		var count int
		result <- db.QueryRow("SELECT COUNT(*) FROM schema_meta").Scan(&count)
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		_ = tx.Rollback()
		err := <-result
		t.Fatalf("fresh read waited for the writer; after release: %v", err)
	}
}
