package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// Bound the number of potentially large per-root JSON payloads; promotion is a
// metadata-only transaction regardless of the number of participating members.
const planStageBatchRows = 100

func (r *Repository) stageOperationRevision(
	ctx context.Context,
	genID, worksetID, kind string,
	p workset.OperationRevisionPersist,
) error {
	if err := r.beginStagingPlan(ctx, genID, worksetID, kind, p); err != nil {
		return err
	}
	for chunk := range slices.Chunk(p.Steps, planStageBatchRows) {
		if err := r.appendStagingPlan(ctx, genID, worksetID, kind, p.PlanID, planRecords{steps: chunk}); err != nil {
			return err
		}
	}
	for chunk := range slices.Chunk(p.Roots, planStageBatchRows) {
		if err := r.appendStagingPlan(ctx, genID, worksetID, kind, p.PlanID, planRecords{roots: chunk}); err != nil {
			return err
		}
	}
	for chunk := range slices.Chunk(p.Components, planStageBatchRows) {
		if err := r.appendStagingPlan(ctx, genID, worksetID, kind, p.PlanID, planRecords{components: chunk}); err != nil {
			return err
		}
	}
	for chunk := range slices.Chunk(p.Conclusions, planStageBatchRows) {
		if err := r.appendStagingPlan(ctx, genID, worksetID, kind, p.PlanID, planRecords{conclusions: chunk}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) beginStagingPlan(
	ctx context.Context,
	genID, worksetID, kind string,
	p workset.OperationRevisionPersist,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO plans
 (plan_id,root_path,scan_root_path,library_id,snapshot_token,status,generation_id,task_kind,task_schema_version,workset_id,
  summary_reason,count_members,count_changed,count_unmet_targets,count_blocked,count_unchanged)
 SELECT ?,?,?,NULLIF(?,''),?,'staging',generation_id,?,?,workset_id,?,?,?,?,?,? FROM plan_generations
 WHERE generation_id=? AND workset_id=? AND operation_type=? AND status='running' AND cancel_requested=0`,
		p.PlanID,
		p.RootPath,
		p.RootPath,
		p.LibraryID,
		p.SnapshotToken,
		kind,
		p.TaskSchemaVersion,
		p.SummaryReason,
		p.Counts.Members,
		p.Counts.Changed,
		p.Counts.UnmetTargets,
		p.Counts.Blocked,
		p.Counts.Unchanged,
		genID,
		worksetID,
		kind,
	)
	if err != nil {
		return fmt.Errorf("begin staging plan: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return workset.ErrGenerationNotFound
	}

	return tx.Commit()
}

func (r *Repository) appendStagingPlan(ctx context.Context, genID, worksetID, kind, planID string, rec planRecords) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := guardStagingPlan(tx, genID, worksetID, kind, planID); err != nil {
		return err
	}
	if err := insertPlanRecords(tx, planID, rec); err != nil {
		return err
	}
	return tx.Commit()
}

// The IMMEDIATE transaction owns the writer before this check. A committed
// cancellation therefore rejects every later chunk and the final promotion.
func guardStagingPlan(tx *sql.Tx, genID, worksetID, kind, planID string) error {
	var writable bool
	err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM plans p JOIN plan_generations g ON g.generation_id=p.generation_id
 WHERE p.plan_id=? AND p.status='staging' AND g.generation_id=? AND g.workset_id=? AND g.operation_type=? AND g.status='running' AND g.cancel_requested=0)`, planID, genID, worksetID, kind).Scan(&writable)
	if err != nil {
		return fmt.Errorf("check staging owner: %w", err)
	}
	if !writable {
		return workset.ErrGenerationNotFound
	}
	return nil
}
