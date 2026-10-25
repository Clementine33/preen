package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/Clementine33/preen/backend/internal/inventory"
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
	session, err := r.scanConnection(sessionID, rootPath)
	if err != nil {
		return inventory.MergeOutcome{}, err
	}
	defer func() { _ = r.closeStagingSession(sessionID) }()
	if rootPath == "" {
		rootPath = session.root
	}
	tx, err := session.conn.BeginTx(context.Background(), nil)
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
	r.catalogChanged()
	return merged, nil
}

// MergeStagingSimple merges a session without reconciling any library's
// bindings. It is the path a member refresh and the storage's own tests take.
func (r *Repository) MergeStagingSimple(scanID, rootPath string) error {
	_, err := r.MergeStaging(scanID, "", rootPath)
	return err
}

// mergeStagingEntries is the set-based merge plus session-scoped stale cleanup
// and, for a full-root scan, the binding reconciliation.
func mergeStagingEntries(tx *sql.Tx, scanID, libraryID, rootPath string) (inventory.MergeOutcome, error) {
	cleanupRoot, scopePath, err := stagingCleanupScope(tx, scanID, rootPath)
	if err != nil {
		return inventory.MergeOutcome{}, err
	}
	if scopePath != "" {
		if err := ensureInventoryDirectory(tx, scopePath, cleanupRoot); err != nil {
			return inventory.MergeOutcome{}, err
		}
	}
	if err := mergeObservedEntries(tx, cleanupRoot); err != nil {
		return inventory.MergeOutcome{}, fmt.Errorf("failed to merge staging entries: %w", err)
	}
	if cleanupRoot != "" {
		if err := cleanupInventoryScope(tx, cleanupRoot, scopePath); err != nil {
			return inventory.MergeOutcome{}, fmt.Errorf("failed to clean stale entries: %w", err)
		}
	}

	// Reconcile the bindings against the surviving directories. Only a
	// full scan is authoritative for a library's work roots; a member
	// refresh leaves every other copy alone.
	var merged int
	if err := tx.QueryRow("SELECT COUNT(*) FROM temp.entries_staging").
		Scan(&merged); err != nil {
		return inventory.MergeOutcome{}, fmt.Errorf("failed to count merged entries: %w", err)
	}
	outcome := inventory.MergeOutcome{Merged: merged}
	if cleanupRoot != "" && scopePath == "" && libraryID != "" {
		bindings, bindErr := syncWorkCopies(tx, libraryID, cleanupRoot)
		if bindErr != nil {
			return inventory.MergeOutcome{}, fmt.Errorf("failed to reconcile work bindings: %w", bindErr)
		}
		outcome.DirsWithMultipleNumbers = bindings.multiple
		outcome.ExampleDirs = bindings.examples
		outcome.ExamplesTruncated = bindings.truncated
	}

	return outcome, nil
}

// stagingCleanupScope resolves the library root and optional folder scope before writes.
func stagingCleanupScope(tx *sql.Tx, scanID, rootPath string) (root, scope string, err error) {
	var sessionRootPath string
	var sessionScopePath sql.NullString
	var sessionKind string
	sessionErr := tx.QueryRow(`
		SELECT root_path, scope_path, kind
		FROM scan_sessions
		WHERE session_id = ?
	`, scanID).Scan(&sessionRootPath, &sessionScopePath, &sessionKind)
	if sessionErr != nil && sessionErr != sql.ErrNoRows {
		return rootPath, "", sessionErr
	}

	if sessionKind == "selected" {
		return rootPath, "", fmt.Errorf("selected scan requires explicit ranges")
	}
	if rootPath == "" {
		rootPath = sessionRootPath
	}

	// Normalize rootPath to POSIX format for consistent SQL matching
	rootPath = filepath.ToSlash(rootPath)

	// Never execute stale cleanup without an explicit root scope
	if rootPath == "" {
		return rootPath, "", nil
	}

	scopePath := ""
	if sessionScopePath.Valid {
		// Normalize scope_path to POSIX as well for consistent matching
		scopePath = filepath.ToSlash(sessionScopePath.String)
	}

	return rootPath, scopePath, nil
}
