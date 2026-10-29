package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DeleteRetiredPlanBatch removes at most limit rows from one table in its own
// transaction. Eligibility is read after acquiring the write lock. Children
// are emptied before their parent, so no delete starts an unbounded cascade.
// No admission slot is needed: retired and abandoned staging data is invisible.
func (r *Repository) DeleteRetiredPlanBatch(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("retired plan batch limit must be positive")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var planID string
	err = tx.QueryRowContext(ctx, `SELECT plan_id FROM plans WHERE `+deletablePlanPredicate+` ORDER BY plan_id LIMIT 1`).
		Scan(&planID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	for _, child := range []struct{ table, predicate string }{
		{"cover_deliveries", "execution_id IN (SELECT execution_id FROM plan_executions WHERE plan_id=?)"},
		{"execution_component_results", "execution_id IN (SELECT execution_id FROM plan_executions WHERE plan_id=?)"},
		{"plan_executions", "plan_id=?"},
		{"conversion_components", "plan_id=?"},
		{"plan_roots", "plan_id=?"},
		{"conversion_steps", "plan_id=?"},
		{"workset_operation_revisions", "plan_id=?"},
		{"plans", "plan_id=?"},
	} {
		var exists bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+child.table+" WHERE "+child.predicate+")", planID).
			Scan(&exists); err != nil {
			return 0, err
		}
		if !exists {
			continue
		}
		result, deleteErr := tx.ExecContext(
			ctx,
			"DELETE FROM "+child.table+" WHERE rowid IN (SELECT rowid FROM "+child.table+" WHERE "+child.predicate+" LIMIT ?) AND EXISTS (SELECT 1 FROM plans WHERE plan_id=? AND "+deletablePlanPredicate+")",
			planID,
			limit,
			planID,
		)
		if deleteErr != nil {
			return 0, deleteErr
		}
		rows, countErr := result.RowsAffected()
		if countErr != nil {
			return 0, countErr
		}
		return rows, tx.Commit()
	}
	return 0, nil
}

// A completed generation retaining staging indicates promotion never committed.
// Running and queued owners are never cleanup candidates, even when canceled.
const deletablePlanPredicate = `(status='retired' OR (status='staging' AND EXISTS (
 SELECT 1 FROM plan_generations g WHERE g.generation_id=plans.generation_id
 AND g.status IN ('failed','canceled','interrupted','completed'))))`
