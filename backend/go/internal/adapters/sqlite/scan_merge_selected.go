package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/librarypath"
)

func (a *ScanStaging) MergeSelectedStaging(
	ctx context.Context,
	id, root string,
	scopes []string,
) (inventory.MergeOutcome, error) {
	return a.repo.MergeSelectedStaging(ctx, id, root, scopes)
}

// MergeSelectedStaging accepts explicit ranges and never derives cleanup scope
// from null session scope. Observed writes and stale cleanup commit together;
// the physical connection then releases staging. Media facts and bindings retain
// the usual merge rules.
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
	session, err := r.scanConnection(id, root)
	if err != nil {
		return inventory.MergeOutcome{}, err
	}
	defer func() { _ = r.closeStagingSession(id) }()
	tx, err := session.conn.BeginTx(ctx, nil)
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
	// Build covered paths once from indexed ranges instead of testing every row against every scope.
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM temp.entries_staging s
		WHERE s.path NOT IN (
			SELECT covered.path FROM json_each(?) scope CROSS JOIN temp.entries_staging covered
			WHERE `+subtreeSQL("covered.path", "scope.value")+`
		)`, string(encodedScopes)).Scan(&outside); err != nil {
		return inventory.MergeOutcome{}, err
	}
	if outside != 0 {
		return inventory.MergeOutcome{}, fmt.Errorf("staging contains entries outside selected ranges")
	}
	for _, scope := range scopes {
		if err := ensureInventoryDirectory(tx, scope, root); err != nil {
			return inventory.MergeOutcome{}, err
		}
	}
	if err := mergeObservedEntries(tx, root); err != nil {
		return inventory.MergeOutcome{}, err
	}
	for _, scope := range scopes {
		if err := cleanupInventoryScope(tx, root, scope); err != nil {
			return inventory.MergeOutcome{}, err
		}
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM temp.entries_staging").
		Scan(&count); err != nil {
		return inventory.MergeOutcome{}, err
	}
	if err := ctx.Err(); err != nil {
		return inventory.MergeOutcome{}, err
	}
	if err := tx.Commit(); err != nil {
		return inventory.MergeOutcome{}, err
	}
	r.catalogChanged()
	return inventory.MergeOutcome{Merged: count}, nil
}
