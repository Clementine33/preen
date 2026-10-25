package sqlite //nolint:testpackage // white-box tests exercise unexported internals

import (
	"database/sql"
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"github.com/Clementine33/preen/backend/internal/inventory"
)

func TestMergeUpdatesOnlyObservedChanges(t *testing.T) {
	for _, tc := range []struct {
		name     string
		root     string
		format   string
		workRoot int
		changed  bool
	}{
		{name: "unchanged", root: "/music"},
		{name: "work_root", root: "/music", workRoot: 1, changed: true},
		{name: "format", root: "/music", format: "flac", changed: true},
		{name: "root_path", root: "/other", changed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, err := NewRepository(t.TempDir() + "/test.db")
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			inventoryfixture.SeedInventory(
				t,
				repo,
				inventoryfixture.Row{
					Path:     "/music/work",
					RootPath: "/music",
					IsDir:    true,
					Size:     10,
					Mtime:    20,
				},
			)
			_, err = repo.db.Exec(`INSERT INTO entries_staging
				(session_id, path, root_path, parent_path, name, is_dir, is_work_root, size, mtime, format)
				VALUES ('new-scan', '/music/work', ?, '/music', 'work', 1, ?, 10, 20, ?)`, tc.root, tc.workRoot, tc.format)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.changed {
				if _, err = repo.db.Exec(
					`CREATE TRIGGER reject_inventory_update BEFORE UPDATE ON entries BEGIN SELECT RAISE(ABORT, 'unchanged inventory rewritten'); END`,
				); err != nil {
					t.Fatal(err)
				}
			}
			outcome, err := repo.MergeStaging("new-scan", "", tc.root)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Merged != 1 {
				t.Fatalf("merged = %d, want 1", outcome.Merged)
			}
			var root string
			var workRoot int
			var format sql.NullString
			err = repo.db.QueryRow(`SELECT root_path, is_work_root, format FROM entries WHERE path = '/music/work'`).
				Scan(&root, &workRoot, &format)
			if err != nil {
				t.Fatal(err)
			}
			wantFormat := sql.NullString{String: tc.format, Valid: tc.format != ""}
			if root != tc.root || workRoot != tc.workRoot || format != wantFormat {
				t.Fatalf("observed fields = %q, %d, %+v; want %q, %d, %+v",
					root, workRoot, format, tc.root, tc.workRoot, wantFormat)
			}
		})
	}
}

func TestMergePreservesBitrateWhenUnchanged(t *testing.T) {
	// Setup: create temp database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()

	// Seed old row in entries with bitrate=320000 (using P0 schema)
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{
			Path:     "/music/test.mp3",
			RootPath: "/music",
			Size:     1024000,
			Mtime:    now,
			Bitrate:  new(int32(320000)),
		},
	)

	// Create staging entry with same size (but no bitrate - tests preservation)
	// The staging entry represents the same file scanned again
	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES ('scan-123', '/music/test.mp3', '/music', '/music', 'test.mp3', 0, 1024000, ?)
	`, now)
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	// Now call merge - this should preserve bitrate because size is unchanged
	// Using type-safe MergeStagingSimple
	err = repo.MergeStagingSimple("scan-123", "/music")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	// Verify: bitrate should be preserved (not null)
	var bitrate sql.NullInt64
	{
		entry := inventoryEntry(t, repo, "/music/test.mp3")
		bitrate = sql.NullInt64{}
		if entry.Bitrate != nil {
			bitrate = sql.NullInt64{Int64: int64(*entry.Bitrate), Valid: true}
		}
	}

	if !bitrate.Valid || bitrate.Int64 != 320000 {
		t.Errorf("expected bitrate=320000 (preserved), got bitrate=%v", bitrate.Int64)
	}

	// Verify: content_rev should be preserved (not incremented)
	var contentRev int
	{
		entry := inventoryEntry(t, repo, "/music/test.mp3")
		contentRev = entry.ContentRev
	}
	if contentRev != 1 {
		t.Errorf("expected content_rev=1 (preserved), got content_rev=%d", contentRev)
	}
}

func TestMergeIncrementsContentRevOnChange(t *testing.T) {
	// Setup: create temp database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	oldMtime := time.Now().Add(-24 * time.Hour).Unix()
	newMtime := time.Now().Unix()

	// Seed old row in entries
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{
			Path:     "/music/test.mp3",
			RootPath: "/music",
			Size:     1024000,
			Mtime:    oldMtime,
			Bitrate:  new(int32(320000)),
		},
	)

	// Create staging entry with changed mtime
	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES ('scan-123', '/music/test.mp3', '/music', '/music', 'test.mp3', 0, 1024000, ?)
	`, newMtime)
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	// Now call merge - content should have changed
	err = repo.MergeStagingSimple("scan-123", "/music")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	// Verify: content_rev should be incremented
	var contentRev int
	{
		entry := inventoryEntry(t, repo, "/music/test.mp3")
		contentRev = entry.ContentRev
	}
	if contentRev != 2 {
		t.Errorf("expected content_rev=2, got content_rev=%d", contentRev)
	}
	entry := inventoryEntry(t, repo, "/music/test.mp3")
	if entry.Bitrate != nil {
		t.Fatalf("changed bitrate was retained: %d", *entry.Bitrate)
	}
}

func TestMergeClearsStagingScope(t *testing.T) {
	// Setup: create temp database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()

	// Create multiple staging entries for scan-123
	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES 
			('scan-123', '/music/a.mp3', '/music', '/music', 'a.mp3', 0, 1024000, ?),
			('scan-123', '/music/b.mp3', '/music', '/music', 'b.mp3', 0, 1024000, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	// Merge with scope scan-123
	err = repo.MergeStagingSimple("scan-123", "/music")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	// Verify: staging entries for scan-123 should be cleared
	var count int
	err = repo.db.QueryRow("SELECT COUNT(*) FROM entries_staging WHERE session_id = ?", "scan-123").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query staging: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 staging entries after merge, got %d", count)
	}
}

func TestMergeAppliesStaleCleanup(t *testing.T) {
	// Setup: create temp database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()

	// Create existing entry that is NOT in staging (stale)
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/music/old.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
	)

	// Create staging entry for different file
	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES ('scan-123', '/music/new.mp3', '/music', '/music', 'new.mp3', 0, 1024000, ?)
	`, now)
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	// Merge with empty scope to trigger full-root stale cleanup
	err = repo.MergeStagingSimple("scan-123", "")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	// Verify: stale entry should be deleted
	var exists int
	{
		found, readErr := repo.RootExistsInInventory("/music/old.mp3")
		if readErr != nil {
			t.Fatal(readErr)
		}
		exists = 0
		if found {
			exists = 1
		}
	}

	if exists != 0 {
		t.Errorf("expected stale entry to be deleted, but it still exists")
	}
}

func TestMergeFullRootCleanupDoesNotTouchOtherRoots(t *testing.T) {
	// Setup: create temp database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()

	// Seed entries in two different roots.
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/music/old.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
		inventoryfixture.Row{Path: "/other/keep.mp3", RootPath: "/other", Size: 1024000, Mtime: now},
	)

	// Stage a new file under /music to drive full-root cleanup there.
	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES ('scan-123', '/music/new.mp3', '/music', '/music', 'new.mp3', 0, 1024000, ?)
	`, now)
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	err = repo.MergeStagingSimple("scan-123", "/music")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	var musicOldExists, otherKeepExists int
	{
		found, readErr := repo.RootExistsInInventory("/music/old.mp3")
		if readErr != nil {
			t.Fatal(readErr)
		}
		musicOldExists = 0
		if found {
			musicOldExists = 1
		}
	}
	{
		found, readErr := repo.RootExistsInInventory("/other/keep.mp3")
		if readErr != nil {
			t.Fatal(readErr)
		}
		otherKeepExists = 0
		if found {
			otherKeepExists = 1
		}
	}

	if musicOldExists != 0 {
		t.Errorf("expected /music/old.mp3 to be deleted during /music cleanup")
	}
	if otherKeepExists != 1 {
		t.Errorf("expected /other/keep.mp3 to remain untouched")
	}
}

func TestMergeNewEntryGetsContentRev1(t *testing.T) {
	// Setup: create temp database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()

	// Create staging entry (no existing entry in entries table)
	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES ('scan-123', '/music/new.mp3', '/music', '/music', 'new.mp3', 0, 1024000, ?)
	`, now)
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	// Merge
	err = repo.MergeStagingSimple("scan-123", "/music")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	// Verify: new entry should have content_rev=1
	var contentRev int
	{
		entry := inventoryEntry(t, repo, "/music/new.mp3")
		contentRev = entry.ContentRev
	}
	if contentRev != 1 {
		t.Errorf("expected content_rev=1 for new entry, got content_rev=%d", contentRev)
	}
}

func TestMergeUnchangedRowPreservesExistingRevision(t *testing.T) {
	// Setup: create temp database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()

	// A later unchanged scan preserves the revision of a previously changed file.
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{
			Path:       "/music/test.mp3",
			RootPath:   "/music",
			Size:       1024000,
			Mtime:      now,
			ContentRev: 2,
		},
	)

	// Create staging entry with same size/mtime (unchanged)
	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES ('scan-123', '/music/test.mp3', '/music', '/music', 'test.mp3', 0, 1024000, ?)
	`, now)
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	// Merge
	err = repo.MergeStagingSimple("scan-123", "/music")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	entry := inventoryEntry(t, repo, "/music/test.mp3")
	if entry.ContentRev != 2 {
		t.Fatalf("unchanged revision = %d", entry.ContentRev)
	}
}

func TestMergePersistsFormatFromStaging(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()

	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime, format)
		VALUES ('scan-123', '/music/new.mp3', '/music', '/music', 'new.mp3', 0, 1024000, ?, 'audio/mpeg')
	`, now)
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	err = repo.MergeStagingSimple("scan-123", "/music")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	var format sql.NullString
	{
		entry := inventoryEntry(t, repo, "/music/new.mp3")
		format = sql.NullString{String: entry.Format, Valid: entry.Format != ""}
	}
	if !format.Valid || format.String != "audio/mpeg" {
		t.Fatalf("expected format audio/mpeg, got %q", format.String)
	}
}

func TestMergeWithExplicitStalePaths(t *testing.T) {
	// Verifies that stalePaths parameter is ignored; preserve behavior only
	// considers entries_staging entries for the session. This test ensures
	// callers are not relying on stalePaths to protect files.

	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()

	// Create existing entries - one in staging, one not
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/music/in_staging.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
		inventoryfixture.Row{Path: "/music/not_in_staging.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
	)

	// Create staging only for in_staging.mp3
	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES ('scan-123', '/music/in_staging.mp3', '/music', '/music', 'in_staging.mp3', 0, 1024000, ?)
	`, now)
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	// Only staging decides what survives: the merge has no "preserve" list at
	// all, so the requested path is stale precisely because staging omitted it.
	err = repo.MergeStagingSimple("scan-123", "/music")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	// Verify: in_staging.mp3 should be preserved (was in staging)
	var keepExists int
	{
		found, readErr := repo.RootExistsInInventory("/music/in_staging.mp3")
		if readErr != nil {
			t.Fatal(readErr)
		}
		keepExists = 0
		if found {
			keepExists = 1
		}
	}
	if keepExists != 1 {
		t.Errorf("expected in_staging.mp3 to exist (in staging)")
	}

	// Verify: not_in_staging.mp3 should be DELETED (stalePaths ignored it)
	var deleteExists int
	{
		found, readErr := repo.RootExistsInInventory("/music/not_in_staging.mp3")
		if readErr != nil {
			t.Fatal(readErr)
		}
		deleteExists = 0
		if found {
			deleteExists = 1
		}
	}
	if deleteExists != 0 {
		t.Errorf("expected not_in_staging.mp3 to be deleted (stalePaths is ignored, not in staging)")
	}
}

func TestMergeScopedCleanupWithEmptyStaging(t *testing.T) {
	// Setup: create temp database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()

	// First create a scan session with a scope (folder scan)
	_, err = repo.db.Exec(`
		INSERT INTO scan_sessions (session_id, root_path, scope_path, kind, status, started_at)
		VALUES ('scan-empty', '/music', '/music/albums', 'folder', 'running', datetime('now'))
	`)
	if err != nil {
		t.Fatalf("failed to create scan session: %v", err)
	}

	// Seed existing entries in the scope that should be cleaned
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/music/albums/old1.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
		inventoryfixture.Row{Path: "/music/albums/old2.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
		inventoryfixture.Row{Path: "/music/other.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
	)

	// Create empty staging (no files scanned)
	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES ('scan-empty', '/music/albums/.placeholder', '/music', '/music/albums', '.placeholder', 0, 0, ?)
	`, now)
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	// Merge with scope - should clean the scope even with minimal staging
	err = repo.MergeStagingSimple("scan-empty", "/music")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	// Verify: scope entries that were NOT in staging should be deleted
	// The placeholder IS in staging so it should remain
	var old1Exists, old2Exists int
	{
		found, readErr := repo.RootExistsInInventory("/music/albums/old1.mp3")
		if readErr != nil {
			t.Fatal(readErr)
		}
		old1Exists = 0
		if found {
			old1Exists = 1
		}
	}
	{
		found, readErr := repo.RootExistsInInventory("/music/albums/old2.mp3")
		if readErr != nil {
			t.Fatal(readErr)
		}
		old2Exists = 0
		if found {
			old2Exists = 1
		}
	}
	if old1Exists != 0 {
		t.Errorf("expected old1.mp3 to be deleted (not in scanned set)")
	}
	if old2Exists != 0 {
		t.Errorf("expected old2.mp3 to be deleted (not in scanned set)")
	}

	// Verify: placeholder should remain (it was in staging/scanned set)
	var placeholderExists int
	{
		found, readErr := repo.RootExistsInInventory("/music/albums/.placeholder")
		if readErr != nil {
			t.Fatal(readErr)
		}
		placeholderExists = 0
		if found {
			placeholderExists = 1
		}
	}
	if placeholderExists != 1 {
		t.Errorf("expected placeholder to remain (in scanned set)")
	}

	// Verify: other entries outside scope should remain
	var otherExists int
	{
		found, readErr := repo.RootExistsInInventory("/music/other.mp3")
		if readErr != nil {
			t.Fatal(readErr)
		}
		otherExists = 0
		if found {
			otherExists = 1
		}
	}
	if otherExists != 1 {
		t.Errorf("expected other.mp3 to remain")
	}
}

func TestMergeBoundarySafeScopeMatching(t *testing.T) {
	// Setup: create temp database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()

	// Create scan session with scope
	_, err = repo.db.Exec(`
		INSERT INTO scan_sessions (session_id, root_path, scope_path, kind, status, started_at)
		VALUES ('scan-boundary', '/music', '/music/albums', 'folder', 'running', datetime('now'))
	`)
	if err != nil {
		t.Fatalf("failed to create scan session: %v", err)
	}

	// Seed entries that should be matched by boundary-safe scope
	// Paths that should be in scope: /music/albums, /music/albums/file.mp3, /music/albums/sub/file.mp3
	// Paths that should NOT be in scope: /music/albums-something, /music/albums2
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/music/albums", RootPath: "/music", IsDir: true, Mtime: now},
		inventoryfixture.Row{Path: "/music/albums/file1.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
		inventoryfixture.Row{Path: "/music/albums/sub/file2.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
		inventoryfixture.Row{
			Path:     "/music/albums-something/should_stay.mp3",
			RootPath: "/music",
			Size:     1024000,
			Mtime:    now,
		},
		inventoryfixture.Row{Path: "/music/albums2/also_stay.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
		inventoryfixture.Row{Path: "/music/other/outside.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
	)

	// Create staging with only file1
	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES ('scan-boundary', '/music/albums/file1.mp3', '/music', '/music/albums', 'file1.mp3', 0, 1024000, ?)
	`, now)
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	// Merge - should only clean within scope boundary
	err = repo.MergeStagingSimple("scan-boundary", "/music")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	for _, want := range []struct {
		path string
		kept bool
	}{
		{"/music/albums/file1.mp3", true},
		{"/music/albums/sub/file2.mp3", false},
		{"/music/albums-something/should_stay.mp3", true},
		{"/music/albums2/also_stay.mp3", true},
		{"/music/other/outside.mp3", true},
	} {
		found, readErr := repo.RootExistsInInventory(want.path)
		if readErr != nil || found != want.kept {
			t.Errorf("entry %s: found=%v want=%v err=%v", want.path, found, want.kept, readErr)
		}
	}
}

func TestMergeWithTrulyEmptyStaging(t *testing.T) {
	// Setup: create temp database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()

	// First create a scan session with a scope (folder scan)
	_, err = repo.db.Exec(`
		INSERT INTO scan_sessions (session_id, root_path, scope_path, kind, status, started_at)
		VALUES ('scan-zero', '/music', '/music/albums', 'folder', 'running', datetime('now'))
	`)
	if err != nil {
		t.Fatalf("failed to create scan session: %v", err)
	}

	// Seed existing entries in the scope
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/music/albums/old1.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
		inventoryfixture.Row{Path: "/music/albums/old2.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
		inventoryfixture.Row{Path: "/music/other.mp3", RootPath: "/music", Size: 1024000, Mtime: now},
	)

	// DO NOT create any staging entries - this tests truly zero-row staging

	// Merge with scope - should clean scope entries since staging is empty
	// (empty preserve list means "delete everything in scope")
	err = repo.MergeStagingSimple("scan-zero", "/music")
	if err != nil {
		t.Fatalf("merge with empty staging should not fail: %v", err)
	}

	// Verify: entries in scope should be deleted (empty staging = clean all in scope)
	var old1Exists, old2Exists int
	{
		found, readErr := repo.RootExistsInInventory("/music/albums/old1.mp3")
		if readErr != nil {
			t.Fatal(readErr)
		}
		old1Exists = 0
		if found {
			old1Exists = 1
		}
	}
	{
		found, readErr := repo.RootExistsInInventory("/music/albums/old2.mp3")
		if readErr != nil {
			t.Fatal(readErr)
		}
		old2Exists = 0
		if found {
			old2Exists = 1
		}
	}
	// With zero staging entries, preservePaths is empty so cleanup removes all in scope
	if old1Exists != 0 {
		t.Errorf("expected old1.mp3 to be deleted (empty staging = clean all in scope)")
	}
	if old2Exists != 0 {
		t.Errorf("expected old2.mp3 to be deleted (empty staging = clean all in scope)")
	}

	// Verify: other entries outside scope should remain
	var otherExists int
	{
		found, readErr := repo.RootExistsInInventory("/music/other.mp3")
		if readErr != nil {
			t.Fatal(readErr)
		}
		otherExists = 0
		if found {
			otherExists = 1
		}
	}
	if otherExists != 1 {
		t.Errorf("expected other.mp3 to remain")
	}
}

// TestMergeLargeSetStaleCleanup tests that stale cleanup works efficiently with large staging sets
// using session-scoped subquery instead of dynamic NOT IN placeholders.
func TestMergeLargeSetStaleCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()

	// Create many existing entries that should be cleaned up (2000+ to test large-set handling)
	for i := range 2100 {
		inventoryfixture.SeedInventory(
			t,
			repo,
			inventoryfixture.Row{
				Path:     fmt.Sprintf("/music/stale%04d.mp3", i),
				RootPath: "/music",
				Size:     1024000,
				Mtime:    now,
			},
		)
	}

	// Create many staging entries (2000+ to test large-set handling)
	for i := range 2100 {
		_, err = repo.db.Exec(`
			INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
			VALUES ('scan-large', ?, '/music', '/music', ?, 0, 1024000, ?)
		`, fmt.Sprintf("/music/new%04d.mp3", i), fmt.Sprintf("new%04d.mp3", i), now)
		if err != nil {
			t.Fatalf("failed to seed staging %d: %v", i, err)
		}
	}

	// Merge should work efficiently without building large NOT IN (?, ?, ...) clause
	err = repo.MergeStagingSimple("scan-large", "/music")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	entries, err := repo.ListEntriesUnderPath("/music")
	if err != nil {
		t.Fatal(err)
	}
	var staleCount, newCount int
	for _, entry := range entries {
		if strings.HasPrefix(entry.Path, "/music/stale") {
			staleCount++
		}
		if strings.HasPrefix(entry.Path, "/music/new") {
			newCount++
		}
	}
	if staleCount != 0 || newCount != 2100 {
		t.Errorf("stale=%d new=%d, want 0/2100", staleCount, newCount)
	}

	// Verify a sample entry has content_rev=1 (new entries)
	var contentRev int
	{
		entry := inventoryEntry(t, repo, "/music/new0000.mp3")
		contentRev = entry.ContentRev
	}
	if contentRev != 1 {
		t.Errorf("expected content_rev=1 for new entry, got %d", contentRev)
	}

	// Verify staging is cleared
	var stagingCount int
	err = repo.db.QueryRow("SELECT COUNT(*) FROM entries_staging WHERE session_id = ?", "scan-large").
		Scan(&stagingCount)
	if err != nil {
		t.Fatalf("failed to query staging count: %v", err)
	}
	if stagingCount != 0 {
		t.Errorf("expected 0 staging entries after merge, got %d", stagingCount)
	}
}

// TestMergeUpsertContentRevIncrementSemantics tests that content_rev increment
// semantics work correctly for both new and existing entries.
//

func TestMergeUpsertContentRevIncrementSemantics(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()
	oldMtime := now - 3600

	// Seed existing entry with content_rev=3
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{
			Path:       "/music/existing.mp3",
			RootPath:   "/music",
			Size:       1024000,
			Mtime:      oldMtime,
			ContentRev: 3,
			Bitrate:    new(int32(320000)),
		},
	)

	// Test 1: New entry should get content_rev=1
	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES ('scan-upsert', '/music/new.mp3', '/music', '/music', 'new.mp3', 0, 1024000, ?)
	`, now)
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	// Test 2: Unchanged existing entry should preserve content_rev
	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES ('scan-upsert', '/music/existing.mp3', '/music', '/music', 'existing.mp3', 0, 1024000, ?)
	`, oldMtime) // Same size and mtime
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	// Test 3: Changed existing entry should increment content_rev and clear bitrate
	changedMtime := now
	_, err = repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES ('scan-upsert', '/music/changed.mp3', '/music', '/music', 'changed.mp3', 0, 2048000, ?)
	`, changedMtime)
	if err != nil {
		t.Fatalf("failed to seed staging: %v", err)
	}

	// Seed existing entry that will be changed
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{
			Path:       "/music/changed.mp3",
			RootPath:   "/music",
			Size:       1024000,
			Mtime:      oldMtime,
			ContentRev: 2,
			Bitrate:    new(int32(256000)),
		},
	)

	err = repo.MergeStagingSimple("scan-upsert", "/music")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	newEntry := inventoryEntry(t, repo, "/music/new.mp3")
	if newEntry.ContentRev != 1 {
		t.Errorf("new revision=%d, want 1", newEntry.ContentRev)
	}
	unchanged := inventoryEntry(t, repo, "/music/existing.mp3")
	if unchanged.ContentRev != 3 || unchanged.Bitrate == nil || *unchanged.Bitrate != 320000 {
		t.Errorf("unchanged facts=%+v", unchanged)
	}
	changed := inventoryEntry(t, repo, "/music/changed.mp3")
	if changed.ContentRev != 3 || changed.Bitrate != nil {
		t.Errorf("changed facts=%+v", changed)
	}
}

// TestMergeScopedCleanupKeepsTheScopeAndItsNeighbours covers the two boundaries
// a folder scan's stale cleanup has to respect: the directory that was scanned
// keeps its own row (a folder scan reports the folder's contents, never the
// folder itself), and a scope whose name contains SQL wildcard characters
// matches only its own subtree — `%` and `_` are ordinary characters in a
// directory name, not patterns.
func TestMergeScopedCleanupKeepsTheScopeAndItsNeighbours(t *testing.T) {
	tmpDir := t.TempDir()
	repo, err := NewRepository(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	if _, err := repo.db.Exec(`
		INSERT INTO scan_sessions (session_id, root_path, scope_path, kind, status, started_at)
		VALUES ('scan-wild', '/music', '/music/100%_hits', 'folder', 'running', datetime('now'))
	`); err != nil {
		t.Fatalf("create scan session: %v", err)
	}

	// The scanned directory, a stale file inside it, a fresh file inside it, and
	// a sibling that a LIKE pattern would read as a match for the scope.
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/music/100%_hits", RootPath: "/music", IsDir: true},
		inventoryfixture.Row{Path: "/music/100%_hits/gone.mp3", RootPath: "/music", Size: 1, Mtime: 1},
		inventoryfixture.Row{Path: "/music/100ZZhits/other.mp3", RootPath: "/music", Size: 1, Mtime: 1},
		inventoryfixture.Row{Path: "/music/album", RootPath: "/music", IsDir: true},
	)
	if _, err := repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime)
		VALUES ('scan-wild', '/music/100%_hits/kept.mp3', '/music', '/music/100%_hits', 'kept.mp3', 0, 1, 1)
	`); err != nil {
		t.Fatalf("seed staging: %v", err)
	}

	if err := repo.MergeStagingSimple("scan-wild", "/music"); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	for _, want := range []struct {
		path string
		kept bool
	}{
		{"/music/100%_hits", true},           // the scanned directory keeps its row
		{"/music/100%_hits/kept.mp3", true},  // …and what the scan saw
		{"/music/100%_hits/gone.mp3", false}, // …what it no longer sees is stale
		{"/music/100ZZhits/other.mp3", true}, // a neighbour the wildcards must not reach
		{"/music/album", true},               // everything else is untouched
	} {
		var count int
		{
			found, readErr := repo.RootExistsInInventory(want.path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			count = 0
			if found {
				count = 1
			}
		}
		if kept := count == 1; kept != want.kept {
			t.Errorf("entries[%s] kept=%v, want %v", want.path, kept, want.kept)
		}
	}
}

// BenchmarkMergeStaging times only merge; set TMPDIR to a disk-backed directory for WAL/fsync measurements.
func BenchmarkMergeStaging(b *testing.B) {
	entries := mergeBenchmarkEntries()
	for _, phase := range []string{"cold", "warm"} {
		b.Run(phase, func(b *testing.B) {
			b.StopTimer()
			for range b.N {
				repo, libraryID := prepareMergeBenchmark(b, entries)
				if phase == "warm" {
					if _, err := repo.MergeStaging("benchmark", libraryID, "/bench/onsei"); err != nil {
						b.Fatal(err)
					}
					stageMergeBenchmark(b, repo, entries)
				}
				b.StartTimer()
				outcome, err := repo.MergeStaging("benchmark", libraryID, "/bench/onsei")
				b.StopTimer()
				if err != nil {
					b.Fatal(err)
				}
				if outcome.Merged != 28839 {
					b.Fatalf("merged=%d, want 28839", outcome.Merged)
				}
				checkMergeBenchmark(b, repo)
				if err = repo.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func mergeBenchmarkEntries() []inventory.StagingEntry {
	const root = "/bench/onsei"
	entries := make([]inventory.StagingEntry, 0, 28839)
	works := make([]string, 940)
	directories := make([]string, 0, 4820)
	for i := range works {
		name := fmt.Sprintf("RJ%08d-%s-%03d", i+1, strings.Repeat("work", 11), i)
		works[i] = path.Join(root, name)
		entries = append(
			entries,
			inventory.StagingEntry{
				SessionID:  "benchmark",
				RootPath:   root,
				Path:       works[i],
				ParentPath: root,
				Name:       name,
				IsDir:      true,
				IsWorkRoot: true,
				Size:       4096,
				Mtime:      1700000000,
			},
		)
	}
	for i := range 4820 {
		name := fmt.Sprintf("disc-%02d", i/940)
		parent := works[i%940]
		directory := path.Join(parent, name)
		directories = append(directories, directory)
		entries = append(
			entries,
			inventory.StagingEntry{
				SessionID:  "benchmark",
				RootPath:   root,
				Path:       directory,
				ParentPath: parent,
				Name:       name,
				IsDir:      true,
				Size:       4096,
				Mtime:      1700000000,
			},
		)
	}
	for i := range 23079 {
		name := fmt.Sprintf("track-%05d-sample.flac", i)
		parent := directories[i%len(directories)]
		entries = append(
			entries,
			inventory.StagingEntry{
				SessionID:  "benchmark",
				RootPath:   root,
				Path:       path.Join(parent, name),
				ParentPath: parent,
				Name:       name,
				Size:       int64(1000000 + i),
				Mtime:      1700000000,
				Format:     "flac",
			},
		)
	}
	return entries
}

func prepareMergeBenchmark(b *testing.B, entries []inventory.StagingEntry) (*Repository, string) {
	b.Helper()
	repo, err := NewDevelopmentRepository(path.Join(b.TempDir(), "merge.db"))
	if err != nil {
		b.Fatal(err)
	}
	library, err := repo.CreateLibrary("Merge benchmark", "/bench/onsei")
	if err != nil {
		repo.Close()
		b.Fatal(err)
	}
	stageMergeBenchmark(b, repo, entries)
	return repo, library.ID
}

func stageMergeBenchmark(b *testing.B, repo *Repository, entries []inventory.StagingEntry) {
	b.Helper()
	store := NewScanStaging(repo)
	for start := 0; start < len(entries); start += defaultBatchSize {
		end := min(start+defaultBatchSize, len(entries))
		if err := store.WriteStagingBatch("benchmark", entries[start:end]); err != nil {
			b.Fatal(err)
		}
	}
}

func checkMergeBenchmark(b *testing.B, repo *Repository) {
	b.Helper()
	entries, err := repo.ListEntriesUnderPath("/bench/onsei")
	if err != nil {
		b.Fatal(err)
	}
	roots, err := repo.ListLibraryWorkRoots("/bench/onsei")
	if err != nil {
		b.Fatal(err)
	}
	var dirs, copies, staging int
	for _, entry := range entries {
		if entry.IsDir {
			dirs++
		}
	}
	rows := len(entries)

	if err = repo.db.QueryRow("SELECT COUNT(*) FROM work_copies").Scan(&copies); err != nil {
		b.Fatal(err)
	}
	if err = repo.db.QueryRow("SELECT COUNT(*) FROM entries_staging").Scan(&staging); err != nil {
		b.Fatal(err)
	}
	if rows != 28839 || dirs != 5760 || len(roots) != 940 || copies != 940 || staging != 0 {
		b.Fatalf(
			"inventory rows=%d dirs=%d roots=%d copies=%d staging=%d",
			rows,
			dirs,
			len(roots),
			copies,
			staging,
		)
	}
}
