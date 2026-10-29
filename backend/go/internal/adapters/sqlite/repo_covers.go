package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

func (r *Repository) EnqueueCovers(ctx context.Context, req coverdelivery.Request) error {
	if len(req.Targets) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var eligible bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM plan_executions e
		JOIN plans p ON p.plan_id=e.plan_id WHERE e.execution_id=? AND p.status NOT IN ('staging','retired'))`, req.ExecutionID).Scan(&eligible); err != nil {
		return err
	}
	if !eligible {
		return nil
	}
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM cover_deliveries WHERE execution_id=? AND work_code=?`, req.ExecutionID, req.WorkCode).
		Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	for _, target := range req.Targets {
		if _, err = tx.ExecContext(
			ctx,
			`INSERT INTO cover_deliveries(id,execution_id,work_code,locale,copy_id,target_json,refresh) VALUES(?,?,?,?,?,?,?)`,
			uuid.NewString(),
			req.ExecutionID,
			req.WorkCode,
			req.Locale,
			target.CopyID,
			coverdelivery.Encode(target),
			req.Refresh,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const coverDestinationColumns = `id,execution_id,target_json,refresh,status,error_code,error_message,published,digest`

// ClaimCover uses implicit rowid allocation on this ordinary TEXT-primary-key table for FIFO.
// Recheck order if IDs become explicit rowids, reach the integer maximum, or the table is rebuilt/WITHOUT ROWID.
func (r *Repository) ClaimCover(ctx context.Context) (*coverdelivery.Job, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`UPDATE cover_deliveries SET status='running' WHERE status='pending' AND work_code=(SELECT work_code FROM cover_deliveries WHERE status='pending' ORDER BY rowid LIMIT 1) RETURNING `+coverDestinationColumns+`,work_code,locale`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var job *coverdelivery.Job
	for rows.Next() {
		var work, locale string
		d, scanErr := scanCoverDestination(rows, &work, &locale)
		if scanErr != nil {
			return nil, scanErr
		}
		if job == nil {
			job = &coverdelivery.Job{WorkCode: work, Locale: locale}
		}
		job.Destinations = append(job.Destinations, d)
	}
	return job, rows.Err()
}

func scanCoverDestination(rows *sql.Rows, extra ...any) (coverdelivery.Destination, error) {
	var d coverdelivery.Destination
	var raw string
	dest := append([]any{
		&d.ID, &d.ExecutionID, &raw, &d.Refresh, &d.Status, &d.ErrorCode, &d.ErrorMessage, &d.Published, &d.Digest,
	}, extra...)
	if err := rows.Scan(dest...); err != nil {
		return d, err
	}
	return d, json.Unmarshal([]byte(raw), &d.Target)
}

func (r *Repository) SaveCoverDestination(ctx context.Context, d coverdelivery.Destination) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE cover_deliveries SET status=?,error_code=?,error_message=?,published=?,digest=? WHERE id=? AND status='running'`,
		d.Status,
		d.ErrorCode,
		d.ErrorMessage,
		d.Published,
		d.Digest,
		d.ID,
	)
	return err
}

func coverIDsPredicate(ids []string) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return `id IN(` + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + `)`, args
}

func (r *Repository) CoverCurrent(ctx context.Context, ids []string) (bool, error) {
	where, args := coverIDsPredicate(ids)
	var running bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cover_deliveries WHERE status='running' AND `+where+`)`, args...).
		Scan(&running)
	return running, err
}

func (r *Repository) FinishCover(ctx context.Context, ids []string) error {
	where, args := coverIDsPredicate(ids)
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE cover_deliveries SET status='pending' WHERE status='running' AND `+where,
		args...)
	return err
}

func (r *Repository) RecoverCovers(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE cover_deliveries SET status='pending' WHERE status='running'`)
	return err
}

func (r *Repository) CoverReport(ctx context.Context, id string) (coverdelivery.Report, error) {
	out := coverdelivery.Report{
		Items:  []coverdelivery.Destination{},
		Counts: map[string]int{"pending": 0, "running": 0, "succeeded": 0, "skipped": 0, "failed": 0, "canceled": 0},
	}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT `+coverDestinationColumns+` FROM cover_deliveries WHERE execution_id=? AND execution_id IN (SELECT e.execution_id FROM plan_executions e JOIN plans p ON p.plan_id=e.plan_id WHERE p.status NOT IN ('staging','retired')) ORDER BY rowid`,
		id,
	)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		d, scanErr := scanCoverDestination(rows)
		if scanErr != nil {
			return out, scanErr
		}
		out.Items = append(out.Items, d)
		out.Counts[d.Status]++
	}
	return out, rows.Err()
}

func (r *Repository) ChangeCoverRequests(ctx context.Context, id string, retry bool) error {
	query := `UPDATE cover_deliveries SET status='canceled',error_code='CANCELED' WHERE execution_id=? AND status IN('pending','running')`
	if retry {
		query = `UPDATE cover_deliveries SET status='pending',error_code='',error_message='' WHERE execution_id=? AND status IN('failed','canceled') AND execution_id IN (SELECT e.execution_id FROM plan_executions e JOIN plans p ON p.plan_id=e.plan_id WHERE p.status NOT IN ('staging','retired'))`
	}
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *Repository) WorkCoverURL(ctx context.Context, code string) (string, error) {
	var url sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT cover_url FROM works WHERE id=?`, code).Scan(&url)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return url.String, err
}

var _ coverdelivery.Store = (*Repository)(nil)
