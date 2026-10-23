package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ==================== Retention Cleanup ====================

// RetentionBatchRows is the batch size used when a caller passes no limit.
const RetentionBatchRows = 500

// deleteScanSessionsBatchTx deletes at most limit terminal scan_sessions rows
// whose COALESCE(finished_at, started_at) is older than cutoff.
//
// Only terminal rows are eligible. A queued/running/merging row belongs to a
// scan that is still in flight, or to one whose process died mid-scan, and the
// database is not the authority on which: deleting on age alone would pull a
// session out from under a running scanner. A row a dead process left behind is
// finalized once at startup (InterruptStaleScanSessions) instead, and becomes
// eligible from the next pass on.
//
// The LIMIT sits in a subquery because this build of SQLite was compiled
// without SQLITE_ENABLE_UPDATE_DELETE_LIMIT: "DELETE ... LIMIT" is a syntax
// error.
func deleteScanSessionsBatchTx(
	ctx context.Context,
	tx *sql.Tx,
	cutoff time.Time,
	limit int,
	candidates ...[]string,
) (int64, error) {
	filter, args := retentionCandidateFilter("session_id", []any{cutoff.Format(timeFormat)}, candidates)
	args = append(args, limit)
	result, err := tx.ExecContext(ctx, `
		DELETE FROM scan_sessions WHERE session_id IN (
			SELECT session_id FROM scan_sessions
			WHERE `+scanRetentionPredicate+filter+`
 LIMIT ?
		)
	`, args...)
	if err != nil {
		return 0, fmt.Errorf("delete scan_sessions older than %s: %w", cutoff.Format(timeFormat), err)
	}
	n, _ := result.RowsAffected()
	return n, nil
}

// deletePlanGenerationsBatchTx purges at most limit planning session rows whose
// finished_at is older than cutoff. finished_at IS NOT NULL is the terminal
// test: a queued or running session has none yet.
//
// revision_id is ON DELETE SET NULL in the other direction - dropping a plan
// clears the session's reference to it - so purging a session never reaches the
// plan it planned.
func deletePlanGenerationsBatchTx(
	ctx context.Context,
	tx *sql.Tx,
	cutoff time.Time,
	limit int,
	candidates ...[]string,
) (int64, error) {
	filter, args := retentionCandidateFilter("generation_id", []any{cutoff.Format(timeFormat)}, candidates)
	args = append(args, limit)
	result, err := tx.ExecContext(ctx, `
		DELETE FROM plan_generations WHERE generation_id IN (
			SELECT generation_id FROM plan_generations
			WHERE `+generationRetentionPredicate+filter+`
 LIMIT ?
		)
	`, args...)
	if err != nil {
		return 0, fmt.Errorf("delete finished plan generations older than %s: %w", cutoff.Format(timeFormat), err)
	}
	n, _ := result.RowsAffected()
	return n, nil
}

// A delivery of any status retains its shared download for reporting and retry.
func deleteUnreferencedCoverDownloadsBatchTx(
	ctx context.Context,
	tx *sql.Tx,
	limit int,
	candidates ...[]string,
) (int64, error) {
	filter, args := retentionCandidateFilter("j.id", nil, candidates)
	args = append(args, limit)
	result, err := tx.ExecContext(ctx, `
		DELETE FROM cover_downloads WHERE id IN (
			SELECT j.id FROM cover_downloads j
			WHERE `+downloadRetentionPredicate+filter+` LIMIT ?
		)
	`, args...)
	if err != nil {
		return 0, fmt.Errorf("delete unreferenced cover downloads: %w", err)
	}
	n, _ := result.RowsAffected()
	return n, nil
}

// RunRetentionCleanupBatch deletes one batch of the rows retention no longer
// keeps, in one transaction, and reports what it deleted.
//
// At most limit rows go per table, so a batch is bounded and each
// one is a short transaction. Batching is what lets the idle-time maintenance
// pass hand the admission slot back to real work between batches: a single
// unbounded delete over a large table would hold the database writer - and
// anyone who needs it - for as long as it takes. A batch that deletes nothing
// means all three tables are clean.
//
// workset.Workset revisions, plans, executions and inventory are never purged here;
// they are durable aggregate history, not session ledgers.
func (r *Repository) RunRetentionCleanupBatch(
	ctx context.Context,
	cutoff, generationCutoff time.Time,
	limit int,
) (CleanupStats, error) {
	if limit <= 0 {
		limit = RetentionBatchRows
	}
	if generationCutoff.IsZero() {
		generationCutoff = cutoff
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return CleanupStats{}, fmt.Errorf("begin retention cleanup tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var stats CleanupStats
	if stats.DeletedScanSessions, err = deleteScanSessionsBatchTx(ctx, tx, cutoff, limit); err != nil {
		return CleanupStats{}, err
	}
	if stats.DeletedGenerations, err = deletePlanGenerationsBatchTx(ctx, tx, generationCutoff, limit); err != nil {
		return CleanupStats{}, err
	}
	if stats.DeletedCoverDownloads, err = deleteUnreferencedCoverDownloadsBatchTx(ctx, tx, limit); err != nil {
		return CleanupStats{}, err
	}

	if err := tx.Commit(); err != nil {
		return CleanupStats{}, fmt.Errorf("commit retention cleanup tx: %w", err)
	}
	return stats, nil
}

const scanRetentionPredicate = "status IN ('completed','failed','canceled','interrupted') AND julianday(COALESCE(finished_at, started_at)) < julianday(?)"
const generationRetentionPredicate = "finished_at IS NOT NULL AND julianday(finished_at) < julianday(?)"

const downloadRetentionPredicate = "j.status='done' AND NOT EXISTS (SELECT 1 FROM cover_deliveries d WHERE d.job_id=j.id)"

// Manual batches bound the existing retention delete to their finite candidates.
func retentionCandidateFilter(column string, args []any, candidates [][]string) (string, []any) {
	if len(candidates) == 0 {
		return "", args
	}
	if len(candidates[0]) == 0 {
		return " AND 0", args
	}
	for _, id := range candidates[0] {
		args = append(args, id)
	}
	return " AND " + column + " IN (" + strings.TrimSuffix(strings.Repeat("?,", len(candidates[0])), ",") + ")", args
}
