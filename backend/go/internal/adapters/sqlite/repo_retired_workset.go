package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// retireWorksetTx revokes cover demand before its publication guard is released.
// Releasing the creation key matches the former synchronous record deletion.
func retireWorksetTx(tx *sql.Tx, id string) error {
	for _, query := range []string{
		`UPDATE worksets SET retired=1,creation_idem_key=NULL WHERE id=?`,
		`UPDATE plans SET status='retired' WHERE workset_id=?`,
		`UPDATE plan_executions SET idempotency_key='' WHERE workset_id=?`,
		`UPDATE cover_deliveries SET status='canceled',error_code='CANCELED'
		 WHERE execution_id IN (SELECT execution_id FROM plan_executions WHERE workset_id=?) AND status IN ('pending','running')`,
	} {
		if _, err := tx.Exec(query, id); err != nil {
			return fmt.Errorf("retire record: %w", err)
		}
	}
	return nil
}

// deleteRetiredWorksetBatchTx runs only after eligible plans have been emptied.
// Delete one child table per transaction so no parent starts a large cascade.
func deleteRetiredWorksetBatchTx(ctx context.Context, tx *sql.Tx, limit int) (int64, error) {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM worksets WHERE retired=1
		AND NOT EXISTS(SELECT 1 FROM plans WHERE workset_id=worksets.id) ORDER BY id LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	for _, table := range []string{"workset_members", "workset_operation_drafts", "plan_generations", "workset_operations", "worksets"} {
		key := "workset_id"
		if table == "worksets" {
			key = "id"
		}
		var exists bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+" WHERE "+key+"=?)", id).
			Scan(&exists); err != nil {
			return 0, err
		}
		if !exists {
			continue
		}
		result, err := tx.ExecContext(
			ctx,
			"DELETE FROM "+table+" WHERE rowid IN (SELECT rowid FROM "+table+" WHERE "+key+"=? LIMIT ?) AND EXISTS(SELECT 1 FROM worksets WHERE id=? AND retired=1)",
			id,
			limit,
			id,
		)
		if err != nil {
			return 0, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		return n, tx.Commit()
	}
	return 0, nil
}
