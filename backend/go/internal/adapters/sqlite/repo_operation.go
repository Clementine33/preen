package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// ==================== workset.Workset operations ====================

const operationColumns = `workset_id, operation_type, version, current_revision_id, created_at, updated_at`

func scanOperation(scanner interface{ Scan(...any) error }) (*workset.Operation, error) {
	var o workset.Operation
	var currentRevision sql.NullString
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&o.WorksetID,
		&o.OperationType,
		&o.Version,
		&currentRevision,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	o.CurrentRevisionID = currentRevision.String
	o.CreatedAt = parseTimestamp(createdAt)
	o.UpdatedAt = parseTimestamp(updatedAt)
	return &o, nil
}

// GetOperation loads one operation. Returns workset.ErrOperationNotFound when the
// (workset, type) pair has no row, which is also the answer for an unknown
// workset: callers load the workset first when they need to distinguish.
func (r *Repository) GetOperation(worksetID, operationType string) (*workset.Operation, error) {
	row := r.db.QueryRow(
		`SELECT `+operationColumns+` FROM workset_operations WHERE workset_id = ? AND operation_type = ?`,
		worksetID,
		operationType,
	)
	o, err := scanOperation(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, workset.ErrOperationNotFound
		}
		return nil, err
	}
	return o, nil
}

// ListOperations returns a workset's operations in creation order.
func (r *Repository) ListOperations(worksetID string) ([]*workset.Operation, error) {
	rows, err := r.db.Query(
		`SELECT `+operationColumns+` FROM workset_operations WHERE workset_id = ? ORDER BY created_at, operation_type`,
		worksetID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*workset.Operation
	for rows.Next() {
		o, scanErr := scanOperation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func insertOperation(tx *sql.Tx, o workset.Operation) error {
	var currentRevision any
	if o.CurrentRevisionID != "" {
		currentRevision = o.CurrentRevisionID
	}
	if _, err := tx.Exec(`
		INSERT INTO workset_operations (workset_id, operation_type, version, current_revision_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, o.WorksetID, o.OperationType, o.Version, currentRevision, o.CreatedAt.Format(timeFormat), o.UpdatedAt.Format(timeFormat)); err != nil {
		return fmt.Errorf("insert operation: %w", err)
	}
	return nil
}

// GetOperationDraft loads one operation's draft. Returns nil when the
// operation has no draft row.
func (r *Repository) GetOperationDraft(worksetID, operationType string) (*workset.OperationDraft, error) {
	var d workset.OperationDraft
	var updatedAt string
	err := r.db.QueryRow(`
		SELECT workset_id, operation_type, schema_version, draft_json, draft_hash, updated_at
		FROM workset_operation_drafts WHERE workset_id = ? AND operation_type = ?
	`, worksetID, operationType).Scan(
		&d.WorksetID,
		&d.OperationType,
		&d.SchemaVersion,
		&d.DraftJSON,
		&d.DraftHash,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	d.UpdatedAt = parseTimestamp(updatedAt)
	return &d, nil
}

// SaveOperationDraft replaces the full draft document and bumps the operation
// version in one transaction. The draft write must never be visible without
// its version bump, so the guarded update and the upsert share one commit.
// workset.ErrVersionConflict on a stale If-Match, workset.ErrOperationNotFound when the
// operation row is gone.
func (r *Repository) SaveOperationDraft(
	worksetID, operationType string,
	schemaVersion int,
	draftJSON, draftHash string,
	expectedVersion int,
	now time.Time,
) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := writeOperationDraftTx(tx, workset.OperationDraft{
		WorksetID:     worksetID,
		OperationType: operationType,
		SchemaVersion: schemaVersion,
		DraftJSON:     draftJSON,
		DraftHash:     draftHash,
		UpdatedAt:     now,
	}, expectedVersion); err != nil {
		return err
	}
	return tx.Commit()
}

// writeOperationDraftTx replaces one operation's draft and advances its
// version in one step, under the If-Match guard the caller echoed.
func writeOperationDraftTx(tx *sql.Tx, d workset.OperationDraft, expectedVersion int) error {
	result, err := tx.Exec(`
		UPDATE workset_operations SET version = version + 1, updated_at = ?
		WHERE workset_id = ? AND operation_type = ? AND version = ?
	`, d.UpdatedAt.Format(timeFormat), d.WorksetID, d.OperationType, expectedVersion)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return operationVersionGuardResult(tx, d.WorksetID, d.OperationType)
	}
	return upsertOperationDraft(tx, d)
}

func upsertOperationDraft(tx *sql.Tx, d workset.OperationDraft) error {
	if _, err := tx.Exec(`
		INSERT INTO workset_operation_drafts (workset_id, operation_type, schema_version, draft_json, draft_hash, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(workset_id, operation_type) DO UPDATE SET
			schema_version = excluded.schema_version,
			draft_json = excluded.draft_json,
			draft_hash = excluded.draft_hash,
			updated_at = excluded.updated_at
	`, d.WorksetID, d.OperationType, d.SchemaVersion, d.DraftJSON, d.DraftHash, d.UpdatedAt.Format(timeFormat)); err != nil {
		return fmt.Errorf("upsert operation draft: %w", err)
	}
	return nil
}

// operationVersionGuardResult distinguishes a stale version from a missing
// operation row.
func operationVersionGuardResult(tx *sql.Tx, worksetID, operationType string) error {
	var n int
	if err := tx.QueryRow(
		"SELECT COUNT(*) FROM workset_operations WHERE workset_id = ? AND operation_type = ?",
		worksetID, operationType,
	).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return workset.ErrOperationNotFound
	}
	return workset.ErrVersionConflict
}

// ==================== workset.Operation revisions ====================

// GetOperationRevision retrieves one revision association by plan id.
func (r *Repository) GetOperationRevision(worksetID, operationType, planID string) (*workset.OperationRevision, error) {
	var rev workset.OperationRevision
	var createdAt string
	err := r.db.QueryRow(`
		SELECT plan_id, workset_id, operation_type, revision_index, draft_hash, member_hash,
		       operation_version, excluded_scope, draft_snapshot, created_at
		FROM workset_operation_revisions WHERE workset_id = ? AND operation_type = ? AND plan_id = ?
		AND plan_id IN (SELECT plan_id FROM plans WHERE status NOT IN ('staging','retired'))
	`, worksetID, operationType, planID).Scan(
		&rev.PlanID,
		&rev.WorksetID,
		&rev.OperationType,
		&rev.RevisionIndex,
		&rev.DraftHash,
		&rev.MemberHash,
		&rev.OperationVersion,
		&rev.ExcludedScope,
		&rev.DraftSnapshot,
		&createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, workset.ErrRevisionNotFound
		}
		return nil, err
	}
	rev.CreatedAt = parseTimestamp(createdAt)
	return &rev, nil
}

// PersistOperationRevision stages bounded payload batches before publishing the
// complete snapshot. Only promotion takes the cover publication guard: the old
// plan remains current throughout staging and on any failure.
func (r *Repository) PersistOperationRevision(
	ctx context.Context,
	genID, worksetID, operationType string,
	now time.Time,
	p workset.OperationRevisionPersist,
) error {
	if err := r.stageOperationRevision(ctx, genID, worksetID, operationType, p); err != nil {
		return err
	}
	write := func() error { return r.promoteOperationRevision(ctx, genID, worksetID, operationType, now, p) }
	if r.PlanRetirementGuard != nil {
		return r.PlanRetirementGuard(write)
	}
	return write()
}

func (r *Repository) promoteOperationRevision(
	ctx context.Context,
	genID, worksetID, operationType string, now time.Time, p workset.OperationRevisionPersist,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin revision promotion tx: %w", err)
	}
	defer tx.Rollback()

	var replacedPlanID sql.NullString
	if readErr := tx.QueryRow(
		"SELECT current_revision_id FROM workset_operations WHERE workset_id = ? AND operation_type = ?",
		worksetID,
		operationType,
	).Scan(&replacedPlanID); readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
		return fmt.Errorf("read current revision: %w", readErr)
	}

	if guardErr := guardStagingPlan(tx, genID, worksetID, operationType, p.PlanID); guardErr != nil {
		return guardErr
	}
	if _, readyErr := tx.Exec(
		`UPDATE plans SET status='ready' WHERE plan_id=? AND status='staging'`, p.PlanID,
	); readyErr != nil {
		return readyErr
	}

	var next int
	if nextErr := tx.QueryRow(
		"SELECT COALESCE(MAX(revision_index), 0) + 1 FROM workset_operation_revisions WHERE workset_id = ? AND operation_type = ?",
		worksetID,
		operationType,
	).Scan(&next); nextErr != nil {
		return fmt.Errorf("next revision index: %w", nextErr)
	}
	if _, revErr := tx.Exec(`
		INSERT INTO workset_operation_revisions (plan_id, workset_id, operation_type, revision_index, draft_hash, member_hash, operation_version, excluded_scope, draft_snapshot, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.PlanID, worksetID, operationType, next, p.DraftHash, p.MemberHash, p.OperationVersion, p.ExcludedScope, p.DraftSnapshot, now.Format(timeFormat)); revErr != nil {
		return fmt.Errorf("insert operation revision: %w", revErr)
	}
	if _, promoteErr := tx.Exec(`
		UPDATE workset_operations SET current_revision_id = ?, version = version + 1, updated_at = ?
		WHERE workset_id = ? AND operation_type = ?
	`, p.PlanID, now.Format(timeFormat), worksetID, operationType); promoteErr != nil {
		return fmt.Errorf("promote operation revision: %w", promoteErr)
	}
	result, err := tx.Exec(`
		UPDATE plan_generations SET status = 'completed', revision_id = ?, finished_at = ?, updated_at = ?
		WHERE generation_id = ? AND status = 'running' AND cancel_requested = 0
	`, p.PlanID, now.Format(timeFormat), now.Format(timeFormat), genID)
	if err != nil {
		return fmt.Errorf("complete generation: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return workset.ErrGenerationNotFound
	}
	retired := replacedPlanID.Valid && replacedPlanID.String != "" && replacedPlanID.String != p.PlanID
	if retired {
		if cleanupErr := retirePlanTx(tx, replacedPlanID.String); cleanupErr != nil {
			return cleanupErr
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if retired && r.OnPlansRetired != nil {
		r.OnPlansRetired()
	}
	return nil
}

// retirePlanTx revokes demand while the caller holds the publication guard.
func retirePlanTx(tx *sql.Tx, planID string) error {
	if _, err := tx.Exec(`UPDATE plans SET status = 'retired' WHERE plan_id = ?`, planID); err != nil {
		return fmt.Errorf("retire plan: %w", err)
	}
	// The hidden execution no longer owns an execution replay key. Release the
	// unique reservation now, just as synchronous deletion previously did.
	if _, err := tx.Exec(`UPDATE plan_executions SET idempotency_key='' WHERE plan_id=?`, planID); err != nil {
		return fmt.Errorf("release retired execution key: %w", err)
	}
	if _, err := tx.Exec(`UPDATE cover_deliveries SET status='canceled',error_code='CANCELED'
		WHERE execution_id IN (SELECT execution_id FROM plan_executions WHERE plan_id=?)
		AND status IN ('pending','running')`, planID); err != nil {
		return fmt.Errorf("cancel retired cover demand: %w", err)
	}
	// Keep the completed session and its replay key, without exposing its old plan.
	if _, err := tx.Exec(`UPDATE plan_generations SET revision_id=NULL WHERE revision_id=?`, planID); err != nil {
		return fmt.Errorf("clear retired generation revision: %w", err)
	}
	return nil
}
