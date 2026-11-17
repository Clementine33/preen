package sqlite_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

func TestPersistBitrateUpdates_ReturnsBeginError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onsei-test-analyze-bitrate-begin-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	repo, err := sqlite.NewRepository(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	repo.Close()

	err = repo.UpdateEntryBitrates([]inventory.BitrateUpdate{{Path: "/scope/a.mp3", BitrateKbps: 128000}}, true)
	if err == nil {
		t.Fatal("expected begin error, got nil")
	}
}

func TestPersistBitrateUpdates_ReturnsExecError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onsei-test-analyze-bitrate-exec-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	repo, err := sqlite.NewRepository(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	if _, dropErr := repo.DB().Exec("DROP TABLE inventory_files"); dropErr != nil {
		t.Fatalf("failed to drop entries table: %v", dropErr)
	}

	_ = repo
	err = repo.UpdateEntryBitrates([]inventory.BitrateUpdate{{Path: "/scope/a.mp3", BitrateKbps: 128000}}, true)
	if err == nil {
		t.Fatal("expected exec error, got nil")
	}
	if !strings.Contains(err.Error(), "no such table") {
		t.Fatalf("expected no such table error, got %v", err)
	}
}

// bitrateUpdateBatchSize mirrors the production per-transaction chunk size, so the
// failing update lands in a later chunk than the first.
const bitrateUpdateBatchSize = 100

func TestPersistBitrateUpdates_RollsBackEarlierChunksOnLaterChunkFailure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onsei-test-analyze-bitrate-atomicity-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	repo, err := sqlite.NewRepository(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	updates := make([]inventory.BitrateUpdate, 0, bitrateUpdateBatchSize+1)
	for i := range bitrateUpdateBatchSize + 1 {
		p := fmt.Sprintf("/scope/%03d.mp3", i)
		inventoryfixture.SeedInventory(
			t,
			repo,
			inventoryfixture.Row{
				Path:     p,
				RootPath: "/scope",
				Size:     1000,
				Format:   "audio/mpeg",
				Mtime:    1234567890,
				Bitrate:  new(int32(0)),
			},
		)
		updates = append(updates, inventory.BitrateUpdate{Path: p, BitrateKbps: 128000})
	}

	failPath := updates[len(updates)-1].Path
	failPathEscaped := strings.ReplaceAll(filepath.Base(failPath), "'", "''")
	triggerSQL := fmt.Sprintf(`
		CREATE TRIGGER fail_second_chunk_update
		BEFORE UPDATE OF bitrate ON inventory_files
		FOR EACH ROW
		WHEN NEW.name = '%s'
		BEGIN
			SELECT RAISE(ABORT, 'forced update failure');
		END;
	`, failPathEscaped)
	if _, triggerErr := repo.DB().Exec(triggerSQL); triggerErr != nil {
		t.Fatalf("failed to create failure trigger: %v", triggerErr)
	}

	_ = repo
	err = repo.UpdateEntryBitrates(updates, true)
	if err == nil {
		t.Fatal("expected persist error, got nil")
	}
	if !strings.Contains(err.Error(), "forced update failure") {
		t.Fatalf("expected forced update failure error, got %v", err)
	}

	entries, readErr := repo.ListEntriesUnderPath("/scope")
	if readErr != nil {
		t.Fatal(readErr)
	}
	var updatedCount int
	for _, entry := range entries {
		if entry.Bitrate != nil && *entry.Bitrate > 0 {
			updatedCount++
		}
	}
	if updatedCount != 0 {
		t.Fatalf("expected all updates rolled back, got %d rows with bitrate > 0", updatedCount)
	}

	var firstPathBitrate int64
	{
		entry := inventoryEntry(t, repo, updates[0].Path)
		firstPathBitrate = 0
		if entry.Bitrate != nil {
			firstPathBitrate = int64(*entry.Bitrate)
		}
	}
	if firstPathBitrate != 0 {
		t.Fatalf("expected earlier-chunk row rollback to 0 bitrate, got %d", firstPathBitrate)
	}
}

func TestPersistBitrateUpdates_ConcurrentSerialization(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onsei-test-bitrate-concurrent-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	repo, err := sqlite.NewRepository(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	// Insert 200 entries with NULL bitrate.
	const totalEntries = 200
	updates := make([]inventory.BitrateUpdate, 0, totalEntries)
	for i := range totalEntries {
		p := fmt.Sprintf("/scope/%03d.mp3", i)
		inventoryfixture.SeedInventory(
			t,
			repo,
			inventoryfixture.Row{Path: p, RootPath: "/scope", Size: 1000, Format: "audio/mpeg", Mtime: 1234567890},
		)
		updates = append(updates, inventory.BitrateUpdate{Path: p, BitrateKbps: 128000})
	}

	// Split into 4 groups and persist concurrently using the same repo.
	const numGoroutines = 4
	perGoroutine := totalEntries / numGoroutines

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)

	for g := range numGoroutines {
		wg.Add(1)
		go func(goroutineIdx int) {
			defer wg.Done()
			start := goroutineIdx * perGoroutine
			end := min(start+perGoroutine, len(updates))
			_ = repo
			if err := repo.UpdateEntryBitrates(updates[start:end], true); err != nil {
				errCh <- err
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent persistBitrateUpdates failed: %v", err)
	}

	// Verify all entries have bitrate set.
	entries, readErr := repo.ListEntriesUnderPath("/scope")
	if readErr != nil {
		t.Fatal(readErr)
	}
	var updatedCount int
	for _, entry := range entries {
		if entry.Bitrate != nil && *entry.Bitrate > 0 {
			updatedCount++
		}
	}
	if updatedCount != totalEntries {
		t.Fatalf("expected %d entries with bitrate > 0, got %d", totalEntries, updatedCount)
	}
}
