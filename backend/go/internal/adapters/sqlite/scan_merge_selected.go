package sqlite

import (
	"context"
	"database/sql"
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

// mergeBatchCost bounds each selected-merge transaction's cost: staged rows plus
// existing inventory rows plus one per folder. A single folder costing more than
// the budget is never split and takes one transaction of its own.
const mergeBatchCost = 2000

// MergeSelectedStaging accepts explicit ranges and never derives cleanup scope
// from null session scope. Scopes are packed into cost-bounded batches and each
// batch commits its observed writes and stale cleanup together: every folder is
// atomic, the whole selection is not. Batches committed before a failure or
// cancellation stay; the physical connection then releases staging. Media facts
// and bindings retain the usual merge rules.
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
	var kind, storedRoot string
	if sessionErr := session.conn.QueryRowContext(ctx, "SELECT kind, root_path FROM scan_sessions WHERE session_id = ?", id).
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
	if err = session.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM temp.entries_staging s
		WHERE s.path NOT IN (
			SELECT covered.path FROM json_each(?) scope CROSS JOIN temp.entries_staging covered
			WHERE `+subtreeSQL("covered.path", "scope.value")+`
		)`, string(encodedScopes)).Scan(&outside); err != nil {
		return inventory.MergeOutcome{}, err
	}
	if outside != 0 {
		return inventory.MergeOutcome{}, fmt.Errorf("staging contains entries outside selected ranges")
	}
	var count int
	if err = session.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM temp.entries_staging").Scan(&count); err != nil {
		return inventory.MergeOutcome{}, err
	}
	costs, err := selectedScopeCosts(ctx, session.conn, root, string(encodedScopes))
	if err != nil {
		return inventory.MergeOutcome{}, err
	}
	committed := 0
	defer func() {
		if committed > 0 {
			r.catalogChanged()
		}
	}()
	for _, batch := range packScopes(scopes, costs, mergeBatchCost) {
		if err = ctx.Err(); err != nil {
			return inventory.MergeOutcome{}, err
		}
		if err = mergeScopeBatch(ctx, session.conn, root, batch); err != nil {
			return inventory.MergeOutcome{}, err
		}
		committed++
		if r.afterMergeBatch != nil {
			r.afterMergeBatch()
		}
	}
	return inventory.MergeOutcome{Merged: count}, nil
}

// selectedScopeCosts prices each scope as its staged rows plus the inventory
// rows cleanup must compare, plus one for the fixed per-scope statements.
// Counts are read without a write transaction and may drift from the merge.
func selectedScopeCosts(ctx context.Context, conn *sql.Conn, root, encodedScopes string) ([]int, error) {
	rows, err := conn.QueryContext(ctx, `SELECT
		(SELECT COUNT(*) FROM temp.entries_staging s WHERE `+subtreeIncludingRootSQL("s.path", "scope.value")+`)
		+ (SELECT COUNT(*) FROM inventory_dirs d WHERE d.root_path = ?1 AND `+subtreeIncludingRootSQL("d.path", "scope.value")+`)
		+ (SELECT COUNT(*) FROM inventory_dirs d JOIN inventory_files f ON f.dir_ref = d.id
			WHERE d.root_path = ?1 AND `+subtreeIncludingRootSQL("d.path", "scope.value")+`)
		+ 1
		FROM json_each(?2) scope ORDER BY scope.key`, root, encodedScopes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var costs []int
	for rows.Next() {
		var cost int
		if err = rows.Scan(&cost); err != nil {
			return nil, err
		}
		costs = append(costs, cost)
	}
	return costs, rows.Err()
}

// packScopes fills batches in order up to budget. A scope is never split; one
// costing more than the budget gets a batch of its own.
func packScopes(scopes []string, costs []int, budget int) [][]string {
	var batches [][]string
	used := 0
	for i, scope := range scopes {
		if len(batches) == 0 || used+costs[i] > budget {
			batches = append(batches, nil)
			used = 0
		}
		batches[len(batches)-1] = append(batches[len(batches)-1], scope)
		used += costs[i]
	}
	return batches
}

// mergeScopeBatch commits one batch's observed writes and stale cleanup together.
func mergeScopeBatch(ctx context.Context, conn *sql.Conn, root string, scopes []string) error {
	encoded, err := json.Marshal(scopes)
	if err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, scope := range scopes {
		if err = ensureInventoryDirectory(tx, scope, root); err != nil {
			return err
		}
	}
	if err = mergeObservedEntries(tx, root, string(encoded)); err != nil {
		return err
	}
	for _, scope := range scopes {
		if err = cleanupInventoryScope(tx, root, scope); err != nil {
			return err
		}
	}
	return tx.Commit()
}
