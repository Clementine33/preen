package sqlite //nolint:testpackage // Lock behavior uses the existing staging execution injection seam.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modernc "modernc.org/sqlite"

	"github.com/Clementine33/preen/backend/internal/inventory"
)

// Staging must leave the main writer free even while its batch transaction is open.
func TestStagingBatchDoesNotLockMainDatabase(t *testing.T) {
	suite := setupTestDB(t)
	var seq int
	var name, path string
	if err := suite.repo.DB().QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	other, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(0)")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	suite.adapter.testExecSeam = func(_ int) error {
		_, err := other.ExecContext(
			context.Background(),
			"INSERT INTO schema_meta(key,value) VALUES('staging-writer','ok') ON CONFLICT(key) DO UPDATE SET value=excluded.value",
		)
		return err
	}
	if err := suite.adapter.WriteStagingEntries(
		"lock",
		[]inventory.StagingEntry{{Path: "/music/file", RootPath: "/music", ParentPath: "/music", Name: "file"}},
	); err != nil {
		t.Fatal(err)
	}
}

var mergeLockFunctionID atomic.Int64

func TestMainDatabaseWriterWaitsForScanMerge(t *testing.T) {
	suite := setupTestDB(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	function := fmt.Sprintf("hold_scan_merge_%d", mergeLockFunctionID.Add(1))
	if err := modernc.RegisterScalarFunction(
		function,
		0,
		func(_ *modernc.FunctionContext, _ []driver.Value) (driver.Value, error) {
			close(entered)
			select {
			case <-release:
			case <-t.Context().Done():
			}
			return int64(1), nil
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := suite.repo.DB().
		Exec("CREATE TRIGGER hold_scan_merge BEFORE INSERT ON inventory_files BEGIN SELECT " + function + "(); END"); err != nil {
		t.Fatal(err)
	}
	if err := suite.adapter.WriteStagingBatch(
		"merge-lock",
		[]inventory.StagingEntry{
			{Path: "/music/track.mp3", RootPath: "/music", ParentPath: "/music", Name: "track.mp3"},
		},
	); err != nil {
		t.Fatal(err)
	}
	merged := make(chan error, 1)
	go func() { _, err := suite.adapter.MergeStaging("merge-lock", "", "/music"); merged <- err }()
	select {
	case <-entered:
	case err := <-merged:
		t.Fatalf("merge ended before lock probe: %v", err)
	}
	other, err := sql.Open("sqlite", suite.repo.dbPath+"?_pragma=busy_timeout(2000)")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	written := make(chan error, 1)
	go func() {
		_, err := other.Exec("INSERT INTO schema_meta(key,value) VALUES('merge-writer','ok')")
		written <- err
	}()
	select {
	case err := <-written:
		t.Fatalf("writer did not wait for merge: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	unblock()
	if err := <-merged; err != nil {
		t.Fatal(err)
	}
	if err := <-written; err != nil {
		t.Fatalf("writer did not resume within busy_timeout: %v", err)
	}
}
