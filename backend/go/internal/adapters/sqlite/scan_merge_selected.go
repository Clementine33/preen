package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/librarypath"
)

func (a *ScanStaging) MergeSelectedStaging(
	ctx context.Context,
	id, root string,
	scopes []string,
) (inventory.MergeOutcome, error) {
	return a.repo.MergeSelectedStaging(ctx, id, root, scopes)
}

// MergeSelectedStaging accepts explicit ranges and never derives cleanup scope
// from null session scope. Staged rows, stale cleanup and staging removal commit
// together; unchanged media facts and all bindings retain the usual merge rules.
func (r *Repository) MergeSelectedStaging(
	ctx context.Context,
	id, root string,
	folders []string,
) (inventory.MergeOutcome, error) {
	scopes, err := inventory.SelectedScopes(root, folders)
	if err != nil || len(scopes) == 0 {
		return inventory.MergeOutcome{}, err
	}
	root = librarypath.CleanRootPath(root)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return inventory.MergeOutcome{}, err
	}
	defer tx.Rollback()
	var kind, storedRoot string
	if sessionErr := tx.QueryRowContext(ctx, "SELECT kind, root_path FROM scan_sessions WHERE session_id = ?", id).
		Scan(&kind, &storedRoot); sessionErr != nil {
		return inventory.MergeOutcome{}, sessionErr
	}
	if kind != "selected" || storedRoot != root {
		return inventory.MergeOutcome{}, fmt.Errorf("selected session or root mismatch")
	}
	// Reject staging outside these explicit scopes before any inventory write.
	encodedScopes, err := json.Marshal(scopes)
	if err != nil {
		return inventory.MergeOutcome{}, err
	}
	var outside int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM entries_staging s
		WHERE s.session_id = ? AND (s.root_path <> ? OR NOT EXISTS (
			SELECT 1 FROM json_each(?) scope
			WHERE s.path >= scope.value || '/' AND s.path < scope.value || '0'
		))`, id, root, string(encodedScopes)).Scan(&outside); err != nil {
		return inventory.MergeOutcome{}, err
	}
	if outside != 0 {
		return inventory.MergeOutcome{}, fmt.Errorf("staging contains entries outside selected ranges")
	}
	if _, err := tx.ExecContext(ctx, mergeObservedEntriesSQL, id, time.Now().Format(timeFormat), id); err != nil {
		return inventory.MergeOutcome{}, err
	}
	for _, scope := range scopes {
		if _, err := tx.ExecContext(ctx, `DELETE FROM entries INDEXED BY sqlite_autoindex_entries_1
			WHERE root_path = ? AND path >= ? AND path < ?
			AND path NOT IN (SELECT path FROM entries_staging WHERE session_id = ?)`, root,
			strings.TrimRight(scope, "/")+"/", strings.TrimRight(scope, "/")+"0", id); err != nil {
			return inventory.MergeOutcome{}, err
		}
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM entries_staging WHERE session_id = ?", id).
		Scan(&count); err != nil {
		return inventory.MergeOutcome{}, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM entries_staging WHERE session_id = ?", id); err != nil {
		return inventory.MergeOutcome{}, err
	}
	if err := ctx.Err(); err != nil {
		return inventory.MergeOutcome{}, err
	}
	if err := tx.Commit(); err != nil {
		return inventory.MergeOutcome{}, err
	}
	return inventory.MergeOutcome{Merged: count}, nil
}
