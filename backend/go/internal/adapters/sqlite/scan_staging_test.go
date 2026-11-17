package sqlite_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

// SQLiteAdapterTestSuite provides integration tests for sqlite.ScanStaging
// to verify staging write batching, rollback semantics, and service path behavior.
type SQLiteAdapterTestSuite struct {
	repo    *sqlite.Repository
	adapter *sqlite.ScanStaging
}

func setupTestDB(t *testing.T) *SQLiteAdapterTestSuite {
	t.Helper()
	sqliteRepo, err := sqlite.NewRepository(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test repository: %v", err)
	}

	adapter := sqlite.NewScanStaging(sqliteRepo)

	t.Cleanup(func() {
		sqliteRepo.Close()
	})

	return &SQLiteAdapterTestSuite{
		repo:    sqliteRepo,
		adapter: adapter,
	}
}

func TestWriteStagingEntries_LargeBatch_SucceedsAtomically(t *testing.T) {
	suite := setupTestDB(t)

	sessionID := "test-large-batch-session"

	// Create 2500 entries (> batchSize of 1000 requires multiple batches)
	var entries []inventory.StagingEntry
	for i := range 2500 {
		entries = append(entries, inventory.StagingEntry{
			SessionID:  sessionID,
			Path:       "/music/album/file" + string(rune('a'+i%26)) + string(rune(i)),
			RootPath:   "/music",
			ParentPath: "/music/album",
			Name:       "file" + string(rune('a'+i%26)) + string(rune(i)),
			IsDir:      false,
			Size:       int64(1000 + i),
			Mtime:      time.Now().Unix(),
			Format:     "audio/mp3",
		})
	}

	// Write should succeed atomically despite batching
	err := suite.adapter.WriteStagingEntries(sessionID, entries)
	if err != nil {
		t.Fatalf("WriteStagingEntries failed for large batch: %v", err)
	}

	// Verify all entries were written
	var count int
	err = sqlite.QueryStagingRow(suite.repo, sessionID, "SELECT COUNT(*) FROM entries_staging").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count staging entries: %v", err)
	}

	if count != 2500 {
		t.Errorf("expected 2500 staging entries, got %d", count)
	}
}

// TestWriteStagingEntries_StagingWriteFailure_TriggersRollbackAndReturnsError
// simulates a mid-write insert failure in WriteStagingEntries and verifies:
// - function returns error
// - transaction rollback occurred (no partial rows for session).
func TestWriteStagingEntries_StagingWriteFailure_TriggersRollbackAndReturnsError(t *testing.T) {
	suite := setupTestDB(t)

	sessionID := "test-rollback-session"
	expectedErr := errors.New("simulated insert failure after 25 execs")

	// Create 50 entries
	var entries []inventory.StagingEntry
	for i := range 50 {
		entries = append(entries, inventory.StagingEntry{
			SessionID:  sessionID,
			Path:       "/music/album/file" + string(rune('a'+i%26)) + string(rune(i)),
			RootPath:   "/music",
			ParentPath: "/music/album",
			Name:       "file" + string(rune('a'+i%26)) + string(rune(i)),
			IsDir:      false,
			Size:       int64(1000 + i),
			Mtime:      time.Now().Unix(),
			Format:     "audio/mp3",
		})
	}

	// Inject failure after 25 exec calls
	sqlite.SetScanStagingExecSeam(suite.adapter, func(callCount int) error {
		if callCount == 25 {
			return expectedErr
		}
		return nil
	})

	// Write should fail with our injected error
	err := suite.adapter.WriteStagingEntries(sessionID, entries)
	if err == nil {
		t.Fatal("expected WriteStagingEntries to return error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error containing %v, got %v", expectedErr, err)
	}

	// Verify rollback occurred - no partial rows should remain
	var count int
	queryErr := sqlite.QueryStagingRow(suite.repo, sessionID, "SELECT COUNT(*) FROM entries_staging").Scan(&count)
	if queryErr != nil {
		t.Fatalf("failed to count staging entries: %v", queryErr)
	}

	if count != 0 {
		t.Errorf("expected 0 staging entries after rollback (transaction should be atomic), got %d", count)
	}
}

// Test empty entries slice.
func TestWriteStagingEntries_EmptyEntries(t *testing.T) {
	suite := setupTestDB(t)

	sessionID := "test-empty"

	// Empty entries should succeed without error
	err := suite.adapter.WriteStagingEntries(sessionID, []inventory.StagingEntry{})
	if err != nil {
		t.Fatalf("WriteStagingEntries failed for empty entries: %v", err)
	}

	// Verify no entries were written
	var count int
	err = sqlite.QueryStagingRow(suite.repo, sessionID, "SELECT COUNT(*) FROM entries_staging").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count entries: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 entries, got %d", count)
	}
}

// Verify that INSERT OR REPLACE behavior is preserved.
func TestWriteStagingEntries_InsertOrReplace(t *testing.T) {
	suite := setupTestDB(t)

	sessionID := "test-upsert"

	// First write
	entries := []inventory.StagingEntry{
		{
			SessionID:  sessionID,
			Path:       "/music/file1.mp3",
			RootPath:   "/music",
			ParentPath: "/music",
			Name:       "file1.mp3",
			IsDir:      false,
			Size:       1000,
			Mtime:      1000,
			Format:     "audio/mpeg",
		},
	}

	err := suite.adapter.WriteStagingEntries(sessionID, entries)
	if err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	// Verify initial write
	var size int64
	err = sqlite.QueryStagingRow(
		suite.repo,
		sessionID,
		"SELECT size FROM entries_staging WHERE path = ?",
		"/music/file1.mp3",
	).Scan(&size)
	if err != nil {
		t.Fatalf("failed to read entry: %v", err)
	}
	if size != 1000 {
		t.Errorf("expected size 1000, got %d", size)
	}

	// Second write with same key but different size (should replace)
	entries[0].Size = 2000
	err = suite.adapter.WriteStagingEntries(sessionID, entries)
	if err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	// Verify entry was replaced, not duplicated
	err = sqlite.QueryStagingRow(
		suite.repo,
		sessionID,
		"SELECT size FROM entries_staging WHERE path = ?",
		"/music/file1.mp3",
	).Scan(&size)
	if err != nil {
		t.Fatalf("failed to read entry after upsert: %v", err)
	}
	if size != 2000 {
		t.Errorf("expected size 2000 after upsert, got %d", size)
	}

	// Verify only one entry exists
	var count int
	err = sqlite.QueryStagingRow(suite.repo, sessionID, "SELECT COUNT(*) FROM entries_staging").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count entries: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 entry after upsert, got %d", count)
	}
}

// TestWriteStagingBatch_MultiTransaction verifies batch writes use separate transactions.
func TestWriteStagingBatch_MultiTransaction(t *testing.T) {
	suite := setupTestDB(t)

	sessionID := "test-multi-tx-batch"

	// First batch
	batch1 := []inventory.StagingEntry{
		{
			SessionID:  sessionID,
			Path:       "/music/file1.mp3",
			RootPath:   "/music",
			ParentPath: "/music",
			Name:       "file1.mp3",
			IsDir:      false,
			Size:       1000,
			Mtime:      1000,
			Format:     "audio/mpeg",
		},
	}

	// Second batch
	batch2 := []inventory.StagingEntry{
		{
			SessionID:  sessionID,
			Path:       "/music/file2.mp3",
			RootPath:   "/music",
			ParentPath: "/music",
			Name:       "file2.mp3",
			IsDir:      false,
			Size:       2000,
			Mtime:      2000,
			Format:     "audio/mpeg",
		},
	}

	// Write both batches
	if err := suite.adapter.WriteStagingBatch(sessionID, batch1); err != nil {
		t.Fatalf("WriteStagingBatch batch1 failed: %v", err)
	}
	if err := suite.adapter.WriteStagingBatch(sessionID, batch2); err != nil {
		t.Fatalf("WriteStagingBatch batch2 failed: %v", err)
	}

	// Verify both batches were written
	var count int
	err := sqlite.QueryStagingRow(suite.repo, sessionID, "SELECT COUNT(*) FROM entries_staging").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count entries: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 entries, got %d", count)
	}
}

// TestCloseStagingSession_PreservesOtherSessions verifies cleanup only affects target session.
func TestCloseStagingSession_PreservesOtherSessions(t *testing.T) {
	suite := setupTestDB(t)

	sessionID1 := "test-cleanup-session1"
	sessionID2 := "test-cleanup-session2"

	// Write entries for both sessions
	entries1 := []inventory.StagingEntry{
		{
			SessionID:  sessionID1,
			Path:       "/music/file1.mp3",
			RootPath:   "/music",
			ParentPath: "/music",
			Name:       "file1.mp3",
			IsDir:      false,
			Size:       1000,
			Mtime:      1000,
			Format:     "audio/mpeg",
		},
	}
	entries2 := []inventory.StagingEntry{
		{
			SessionID:  sessionID2,
			Path:       "/music/file2.mp3",
			RootPath:   "/music",
			ParentPath: "/music",
			Name:       "file2.mp3",
			IsDir:      false,
			Size:       2000,
			Mtime:      2000,
			Format:     "audio/mpeg",
		},
	}

	if err := suite.adapter.WriteStagingEntries(sessionID1, entries1); err != nil {
		t.Fatalf("WriteStagingEntries for session1 failed: %v", err)
	}
	if err := suite.adapter.WriteStagingEntries(sessionID2, entries2); err != nil {
		t.Fatalf("WriteStagingEntries for session2 failed: %v", err)
	}

	// Cleanup only session1
	if err := suite.adapter.CloseStagingSession(sessionID1); err != nil {
		t.Fatalf("CloseStagingSession failed: %v", err)
	}

	// Verify session1 entries removed
	var count1, count2 int
	err := sqlite.QueryStagingRow(suite.repo, sessionID1, "SELECT COUNT(*) FROM entries_staging").Scan(&count1)
	if err != nil {
		t.Fatalf("failed to count session1 entries: %v", err)
	}
	if count1 != 0 {
		t.Errorf("expected 0 entries for session1, got %d", count1)
	}

	// Verify session2 entries preserved
	err = sqlite.QueryStagingRow(suite.repo, sessionID2, "SELECT COUNT(*) FROM entries_staging").Scan(&count2)
	if err != nil {
		t.Fatalf("failed to count session2 entries: %v", err)
	}
	if count2 != 1 {
		t.Errorf("expected 1 entry for session2, got %d", count2)
	}
}

// Benchmark for large batch performance.
func BenchmarkWriteStagingEntries_LargeBatch(b *testing.B) {
	sqliteRepo, err := sqlite.NewRepository(b.TempDir() + "/test.db")
	if err != nil {
		b.Fatalf("failed to create test repository: %v", err)
	}
	defer sqliteRepo.Close()

	adapter := sqlite.NewScanStaging(sqliteRepo)

	// Create 5000 entries
	var entries []inventory.StagingEntry
	for i := range 5000 {
		entries = append(entries, inventory.StagingEntry{
			SessionID:  "bench-session",
			Path:       "/music/album/file" + string(rune('a'+i%26)) + string(rune(i)),
			RootPath:   "/music",
			ParentPath: "/music/album",
			Name:       "file" + string(rune('a'+i%26)) + string(rune(i)),
			IsDir:      false,
			Size:       int64(1000 + i),
			Mtime:      time.Now().Unix(),
			Format:     "audio/mp3",
		})
	}

	b.ResetTimer()
	for i := range b.N {
		err := adapter.WriteStagingEntries("bench-session-"+string(rune(i)), entries)
		if err != nil {
			b.Fatalf("WriteStagingEntries failed: %v", err)
		}
	}
}
