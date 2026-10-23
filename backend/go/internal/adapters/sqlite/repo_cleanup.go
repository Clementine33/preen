package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Clementine33/preen/backend/internal/cleanup"
)

// CleanupCandidates captures a finite stage. Preview projects only eligible works
// in its selection; execution uses current references after the works stage.
func (r *Repository) CleanupCandidates(
	ctx context.Context,
	category cleanup.Category,
	cutoff time.Time,
	selected []cleanup.Category,
) ([]string, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var query string
	var args []any
	switch category {
	case cleanup.Works:
		return cleanupWorks(ctx, tx)
	case cleanup.ScanSessions:
		query = "SELECT session_id FROM scan_sessions WHERE " + scanRetentionPredicate
		args = []any{cutoff.Format(timeFormat)}
	case cleanup.PlanGenerations:
		query = "SELECT generation_id FROM plan_generations WHERE " + generationRetentionPredicate
		args = []any{cutoff.Format(timeFormat)}
	case cleanup.CoverDownloads:
		query = "SELECT j.id FROM cover_downloads j WHERE " + downloadRetentionPredicate
	case cleanup.SourceTags, cleanup.SourceVoiceActors, cleanup.SourceCircles:
		table, reference := cleanupSource(category)
		projected := []string{}
		if slices.Contains(selected, cleanup.Works) {
			projected, err = cleanupWorks(ctx, tx)
			if err != nil {
				return nil, err
			}
		}
		data, marshalErr := json.Marshal(projected)
		if marshalErr != nil {
			return nil, marshalErr
		}
		query = "SELECT id FROM " + table + " WHERE work_id IS NULL AND NOT EXISTS (" + reference + " AND j.work_id NOT IN (SELECT value FROM json_each(?)))"
		args = []any{string(data)}
	default:
		return nil, cleanup.ErrInvalidCategories
	}
	return cleanupIDs(ctx, tx, query, args...)
}

func (r *Repository) CreateCleanupRun(ctx context.Context, run cleanup.Run) error {
	data, err := json.Marshal(run)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(
		ctx,
		"INSERT INTO cleanup_run(slot,summary) VALUES(1,?) ON CONFLICT(slot) DO UPDATE SET summary=excluded.summary",
		string(data),
	)
	return err
}
func (r *Repository) ReadCleanupRun(ctx context.Context) (*cleanup.Run, error) {
	var data string
	err := r.db.QueryRowContext(ctx, "SELECT summary FROM cleanup_run WHERE slot=1").Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var run cleanup.Run
	if decodeErr := json.Unmarshal([]byte(data), &run); decodeErr != nil {
		return nil, decodeErr
	}
	return &run, nil
}

// CleanupBatch rechecks eligibility and commits actual counts with the deletion.
func (r *Repository) CleanupBatch(
	ctx context.Context,
	id string,
	category cleanup.Category,
	cutoff time.Time,
	ids []string,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var deleted int64
	switch category {
	case cleanup.ScanSessions:
		deleted, err = deleteScanSessionsBatchTx(ctx, tx, cutoff, len(ids), ids)
	case cleanup.PlanGenerations:
		deleted, err = deletePlanGenerationsBatchTx(ctx, tx, cutoff, len(ids), ids)
	case cleanup.CoverDownloads:
		deleted, err = deleteUnreferencedCoverDownloadsBatchTx(ctx, tx, len(ids), ids)
	case cleanup.Works:
		deleted, err = deleteCleanupWorks(ctx, tx, ids)
	case cleanup.SourceTags, cleanup.SourceVoiceActors, cleanup.SourceCircles:
		deleted, err = deleteCleanupSources(ctx, tx, category, ids)
	default:
		return cleanup.ErrInvalidCategories
	}
	if err != nil {
		return err
	}
	prefix := "$.counts." + string(category)
	result, err := tx.ExecContext(
		ctx,
		`UPDATE cleanup_run SET summary=json_set(summary,
 '$.phase',?, ? , COALESCE(json_extract(summary,?),0)+?,
 ? , COALESCE(json_extract(summary,?),0)+?,
 ? , COALESCE(json_extract(summary,?),0)+?)
 WHERE slot=1 AND json_extract(summary,'$.run_id')=? AND json_extract(summary,'$.status') IN ('running','stopping')`,
		string(
			category,
		),
		prefix+".checked",
		prefix+".checked",
		len(ids),
		prefix+".deleted",
		prefix+".deleted",
		deleted,
		prefix+".skipped",
		prefix+".skipped",
		int64(len(ids))-deleted,
		id,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return cleanup.ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if deleted > 0 &&
		(category == cleanup.Works || category == cleanup.SourceTags || category == cleanup.SourceVoiceActors || category == cleanup.SourceCircles) {
		r.catalogChanged()
	}
	return nil
}
func (r *Repository) StopCleanupRun(ctx context.Context, id string) (*cleanup.Run, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var data string
	err = tx.QueryRowContext(ctx, `SELECT summary FROM cleanup_run WHERE slot=1 AND json_extract(summary,'$.run_id')=?`, id).
		Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, cleanup.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var run cleanup.Run
	if decodeErr := json.Unmarshal([]byte(data), &run); decodeErr != nil {
		return nil, decodeErr
	}
	if run.Status == "running" {
		_, err = tx.ExecContext(
			ctx,
			`UPDATE cleanup_run SET summary=json_set(summary,'$.status','stopping') WHERE slot=1`,
		)
		if err != nil {
			return nil, err
		}
		run.Status = "stopping"
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &run, nil
}
func (r *Repository) FinishCleanupRun(ctx context.Context, id, status, reason string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE cleanup_run SET summary=json_set(summary,
 '$.status',CASE WHEN json_extract(summary,'$.status')='stopping' THEN 'stopped' ELSE ? END,
 '$.reason',CASE WHEN json_extract(summary,'$.status')='stopping' THEN 'Stop requested.' ELSE ? END,
 '$.finished_at',?) WHERE slot=1 AND json_extract(summary,'$.run_id')=? AND json_extract(summary,'$.status') IN ('running','stopping')`, status, reason, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}
func (r *Repository) InterruptCleanupRun(ctx context.Context) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE cleanup_run SET summary=json_set(summary,'$.status','interrupted','$.reason','Service restarted.','$.finished_at',?) WHERE json_extract(summary,'$.status') IN ('running','stopping')`,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("interrupt stale cleanup: %w", err)
	}
	return nil
}

func (r *Repository) SetCleanupPhase(ctx context.Context, id string, category cleanup.Category) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE cleanup_run SET summary=json_set(summary,'$.phase',?) WHERE slot=1 AND json_extract(summary,'$.run_id')=? AND json_extract(summary,'$.status') IN ('running','stopping')`,
		string(category),
		id,
	)
	return err
}
