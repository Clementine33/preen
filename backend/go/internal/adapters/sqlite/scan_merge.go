package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/pathnorm"
)

// MergeStaging merges one scan session's staged entries into the inventory and
// reconciles the library's directory bindings, all in one transaction.
//
// The binding work belongs here rather than at the call site because it must
// not be separable from the merge: the reconciliation reads the directories the
// merge just left behind, so doing it anywhere else would let the inventory
// change and the bindings not.
//
// libraryID may be empty, which is the case for a member refresh: a refresh
// does not reconcile the library's work roots and must not touch another
// copy's binding.
func (r *Repository) MergeStaging(sessionID, libraryID, rootPath string) (inventory.MergeOutcome, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return inventory.MergeOutcome{}, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	merged, err := mergeStagingEntries(tx, sessionID, libraryID, rootPath)
	if err != nil {
		return inventory.MergeOutcome{}, err
	}
	if err := tx.Commit(); err != nil {
		return inventory.MergeOutcome{}, err
	}
	return merged, nil
}

// MergeStagingSimple merges a session without reconciling any library's
// bindings. It is the path a member refresh and the storage's own tests take.
func (r *Repository) MergeStagingSimple(scanID, rootPath string) error {
	_, err := r.MergeStaging(scanID, "", rootPath)
	return err
}

const mergeObservedEntriesSQL = `
		INSERT INTO entries (
			path, root_path, parent_path, name, is_dir, is_work_root, size, mtime,
			scan_id, content_rev, dirty_flag, bitrate, width, height, duration_sec, format, updated_at
		)
		SELECT
			s.path,
			s.root_path,
			s.parent_path,
			s.name,
			s.is_dir,
			s.is_work_root,
			s.size,
			s.mtime,
			?,
			CASE
				WHEN e.path IS NULL THEN 1
				WHEN e.size IS NOT s.size OR e.mtime IS NOT s.mtime OR e.is_dir IS NOT s.is_dir THEN e.content_rev + 1
				ELSE e.content_rev
			END,
CASE
			WHEN e.path IS NULL THEN 0
			WHEN e.size IS NOT s.size OR e.mtime IS NOT s.mtime OR e.is_dir IS NOT s.is_dir THEN 1
			ELSE 0
		END,
			CASE
			WHEN e.path IS NULL THEN NULL
			WHEN e.size IS NOT s.size OR e.mtime IS NOT s.mtime OR e.is_dir IS NOT s.is_dir THEN NULL
			ELSE e.bitrate
		END,
			CASE
			WHEN e.path IS NULL THEN NULL
			WHEN e.size IS NOT s.size OR e.mtime IS NOT s.mtime OR e.is_dir IS NOT s.is_dir THEN NULL
			ELSE e.width
		END,
			CASE
			WHEN e.path IS NULL THEN NULL
			WHEN e.size IS NOT s.size OR e.mtime IS NOT s.mtime OR e.is_dir IS NOT s.is_dir THEN NULL
			ELSE e.height
		END,
			CASE
			WHEN e.path IS NULL THEN NULL
			WHEN e.size IS NOT s.size OR e.mtime IS NOT s.mtime OR e.is_dir IS NOT s.is_dir THEN NULL
			ELSE e.duration_sec
		END,
			NULLIF(s.format, ''),
			?
		FROM entries_staging s
		LEFT JOIN entries e ON s.path = e.path
		WHERE s.session_id = ?
		ON CONFLICT(path) DO UPDATE SET
			root_path = excluded.root_path,
			parent_path = excluded.parent_path,
			name = excluded.name,
			is_dir = excluded.is_dir,
			is_work_root = excluded.is_work_root,
			size = excluded.size,
			mtime = excluded.mtime,
			scan_id = excluded.scan_id,
			content_rev = excluded.content_rev,
			dirty_flag = excluded.dirty_flag,
			bitrate = excluded.bitrate,
			width = excluded.width,
			height = excluded.height,
			duration_sec = excluded.duration_sec,
			format = excluded.format,
			updated_at = excluded.updated_at
            WHERE entries.root_path IS NOT excluded.root_path
               OR entries.parent_path IS NOT excluded.parent_path
               OR entries.name IS NOT excluded.name
               OR entries.is_dir IS NOT excluded.is_dir
               OR entries.is_work_root IS NOT excluded.is_work_root
               OR entries.size IS NOT excluded.size
               OR entries.mtime IS NOT excluded.mtime
               OR entries.format IS NOT excluded.format
               OR entries.dirty_flag IS NOT excluded.dirty_flag
	`

// mergeStagingEntries is the set-based merge plus session-scoped stale cleanup
// and, for a full-root scan, the binding reconciliation.
func mergeStagingEntries(tx *sql.Tx, scanID, libraryID, rootPath string) (inventory.MergeOutcome, error) {
	now := time.Now()

	// Step 1: Set-based merge with content_rev lifecycle logic
	// Using INSERT ... SELECT ... ON CONFLICT for efficient batch processing
	// Semantics:
	// - new row => content_rev=1, dirty_flag=0
	// - changed size/mtime/type => increment content_rev, dirty_flag=1, bitrate and
	//   the media facts (width/height/duration, ADR 0015 §2) cleared
	// - unchanged size/mtime/type => preserve content_rev, bitrate and media facts
	_, err := tx.Exec(mergeObservedEntriesSQL, scanID, now.Format(timeFormat), scanID)
	if err != nil {
		return inventory.MergeOutcome{}, fmt.Errorf("failed to merge staging entries: %w", err)
	}

	// Step 2: Apply stale cleanup (always scoped to a root). Its failure is
	// reported rather than dropped: a half-cleaned inventory is exactly the
	// state the merge is supposed to make impossible, so the transaction has to
	// roll back while it still can.
	cleanupRoot, fullRoot, err := cleanupStaleEntries(tx, scanID, rootPath)
	if err != nil {
		return inventory.MergeOutcome{}, fmt.Errorf("failed to clean stale entries: %w", err)
	}

	// Step 3: Reconcile the bindings against the surviving directories. Only a
	// full scan is authoritative for a library's work roots; a member
	// refresh leaves every other copy alone.
	var merged int
	if err := tx.QueryRow("SELECT COUNT(*) FROM entries_staging WHERE session_id = ?", scanID).
		Scan(&merged); err != nil {
		return inventory.MergeOutcome{}, fmt.Errorf("failed to count merged entries: %w", err)
	}
	outcome := inventory.MergeOutcome{Merged: merged}
	if fullRoot && libraryID != "" {
		bindings, bindErr := syncWorkCopies(tx, libraryID, cleanupRoot)
		if bindErr != nil {
			return inventory.MergeOutcome{}, fmt.Errorf("failed to reconcile work bindings: %w", bindErr)
		}
		outcome.DirsWithoutNumber = bindings.unnumbered
		outcome.DirsWithMultipleNumbers = bindings.multiple
		outcome.ExampleDirs = bindings.examples
		outcome.ExamplesTruncated = bindings.truncated
	}

	// Step 4: Clear staging for this session
	if _, err := tx.Exec("DELETE FROM entries_staging WHERE session_id = ?", scanID); err != nil {
		return inventory.MergeOutcome{}, fmt.Errorf("failed to clear staging: %w", err)
	}

	return outcome, nil
}

// cleanupStaleEntries resolves the effective cleanup root (session scope or
// staging-derived) and deletes stale entries not present in this session's
// staging set. It reports the normalized root it used and whether the cleanup
// covered the whole root rather than one member scope.
func cleanupStaleEntries(tx *sql.Tx, scanID, rootPath string) (root string, fullRoot bool, err error) {
	var sessionRootPath string
	var sessionScopePath sql.NullString
	sessionErr := tx.QueryRow(`
		SELECT root_path, scope_path
		FROM scan_sessions
		WHERE session_id = ?
	`, scanID).Scan(&sessionRootPath, &sessionScopePath)
	if sessionErr != nil && sessionErr != sql.ErrNoRows {
		return rootPath, false, sessionErr
	}

	if rootPath == "" {
		rootPath = sessionRootPath
	}

	// If still no rootPath, derive from staging entries
	if rootPath == "" {
		var stagingRootPath string
		deriveErr := tx.QueryRow(`
			SELECT root_path FROM entries_staging WHERE session_id = ? LIMIT 1
		`, scanID).Scan(&stagingRootPath)
		if deriveErr != nil && !errors.Is(deriveErr, sql.ErrNoRows) {
			return rootPath, false, deriveErr
		}
		rootPath = stagingRootPath
	}

	// Normalize rootPath to POSIX format for consistent SQL matching
	rootPath = filepath.ToSlash(rootPath)

	// Never execute stale cleanup without an explicit root scope
	if rootPath == "" {
		return rootPath, false, nil
	}

	scopePath := ""
	if sessionScopePath.Valid {
		// Normalize scope_path to POSIX as well for consistent matching
		scopePath = filepath.ToSlash(sessionScopePath.String)
	}

	if scopePath == "" {
		// Full-root cleanup: DELETE ... WHERE root_path=? AND path NOT IN
		// (SELECT path FROM entries_staging WHERE session_id=?)
		if _, err := tx.Exec(`
			DELETE FROM entries
			WHERE root_path = ?
				AND path NOT IN (SELECT path FROM entries_staging WHERE session_id = ?)
		`, rootPath, scanID); err != nil {
			return rootPath, false, err
		}
		return rootPath, true, nil
	}

	// Scoped cleanup: everything *under* the scope that the scan of that
	// scope no longer reports is stale.
	//
	// The scope directory's own row is deliberately outside that set: a
	// folder scan reports the folder's contents (the walk emits the
	// scope's children), so the scope row can never appear in staging,
	// and treating its absence as stale would unlist the directory that
	// was just scanned — the overview listing and the directory identity
	// the workbench addresses both read that row.
	//
	// Binary path ranges preserve slash boundaries and treat `%` and `_`
	// literally. Pin the primary path index: without this hint SQLite can
	// choose root_path alone and visit the entire library for a member refresh.
	if _, err := tx.Exec(`
		DELETE FROM entries
		WHERE root_path = ?
			AND path >= ? AND path < ? AND path <> ?
			AND path NOT IN (SELECT path FROM entries_staging WHERE session_id = ?)
	`, rootPath, strings.TrimRight(scopePath, "/")+"/", strings.TrimRight(scopePath, "/")+"0", pathnorm.CleanRootPath(scopePath), scanID); err != nil {
		return rootPath, false, err
	}
	return rootPath, false, nil
}
