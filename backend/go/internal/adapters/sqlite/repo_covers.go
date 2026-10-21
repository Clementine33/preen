package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

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
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM cover_deliveries d JOIN cover_downloads j ON j.id=d.job_id WHERE d.execution_id=? AND j.work_code=?`, req.ExecutionID, req.WorkCode).
		Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	var version int64
	var oldURL, oldBackup string
	err = tx.QueryRowContext(ctx, `SELECT url,backup_url,version FROM cover_candidates WHERE work_code=?`, req.WorkCode).
		Scan(&oldURL, &oldBackup, &version)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if req.URL == "" && !req.Refresh && !req.MainChecked {
		req.URL = oldURL
	}
	if req.BackupURL == "" && !req.Refresh && !req.BackupChecked {
		req.BackupURL = oldBackup
	}
	if version == 0 || oldURL != req.URL || oldBackup != req.BackupURL || req.Refresh {
		version++
		if _, err = tx.ExecContext(
			ctx,
			`INSERT INTO cover_candidates VALUES(?,?,?,?) ON CONFLICT(work_code) DO UPDATE SET url=excluded.url,backup_url=excluded.backup_url,version=excluded.version`,
			req.WorkCode,
			req.URL,
			req.BackupURL,
			version,
		); err != nil {
			return err
		}
		if _, err = tx.ExecContext(
			ctx,
			`UPDATE cover_deliveries SET status='canceled',error_code='SOURCE_UPDATED' WHERE status='pending' AND job_id IN(SELECT id FROM cover_downloads WHERE work_code=? AND version<>?)`,
			req.WorkCode,
			version,
		); err != nil {
			return err
		}
	}
	id := uuid.NewString()
	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO cover_downloads(id,work_code,version,url,backup_url,locale,refresh,main_checked,backup_checked) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(work_code,version) DO UPDATE SET status=CASE WHEN cover_downloads.status IN('done','failed') THEN 'queued' ELSE cover_downloads.status END,main_checked=MAX(cover_downloads.main_checked,excluded.main_checked),backup_checked=MAX(cover_downloads.backup_checked,excluded.backup_checked)`,
		id,
		req.WorkCode,
		version,
		req.URL,
		req.BackupURL,
		req.Locale,
		req.Refresh,
		req.MainChecked,
		req.BackupChecked,
	); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT id FROM cover_downloads WHERE work_code=? AND version=?`, req.WorkCode, version).
		Scan(&id); err != nil {
		return err
	}
	for _, target := range req.Targets {
		if _, err = tx.ExecContext(
			ctx,
			`INSERT INTO cover_deliveries(id,job_id,execution_id,copy_id,target_json,refresh) VALUES(?,?,?,?,?,?)`,
			uuid.NewString(),
			id,
			req.ExecutionID,
			target.CopyID,
			coverdelivery.Encode(target),
			req.Refresh,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const coverJobColumns = `id,work_code,url,backup_url,locale,refresh,version,stage_path,extension,digest,etag,last_modified,main_checked,backup_checked`

// ClaimCover uses implicit rowid allocation on this ordinary TEXT-primary-key table for FIFO.
// Recheck order if IDs become explicit rowids, reach the integer maximum, or the table is rebuilt/WITHOUT ROWID.
func (r *Repository) ClaimCover(ctx context.Context) (*coverdelivery.Job, error) {
	var j coverdelivery.Job
	err := r.db.QueryRowContext(ctx, `UPDATE cover_downloads SET status='running' WHERE id=(SELECT id FROM cover_downloads WHERE status='queued' ORDER BY rowid LIMIT 1) AND status='queued' RETURNING `+coverJobColumns).
		Scan(&j.ID, &j.WorkCode, &j.URL, &j.BackupURL, &j.Locale, &j.Refresh, &j.Version, &j.StagePath, &j.Extension, &j.Digest, &j.ETag, &j.LastModified, &j.MainChecked, &j.BackupChecked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &j, err
}
func (r *Repository) SaveCoverStage(ctx context.Context, j coverdelivery.Job) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE cover_downloads SET url=?,backup_url=?,stage_path=?,extension=?,digest=?,etag=?,last_modified=?,main_checked=?,backup_checked=? WHERE id=? AND status='running'`,
		j.URL,
		j.BackupURL,
		j.StagePath,
		j.Extension,
		j.Digest,
		j.ETag,
		j.LastModified,
		j.MainChecked,
		j.BackupChecked,
		j.ID,
	)
	return err
}
func (r *Repository) CoverDestinations(ctx context.Context, id string) ([]coverdelivery.Destination, error) {
	return r.coverDestinations(ctx, `d.job_id=?`, id, false)
}

func (r *Repository) coverDestinations(
	ctx context.Context,
	where string,
	id string,
	running bool,
) ([]coverdelivery.Destination, error) {
	status := "d.status"
	if running {
		status = "CASE WHEN d.status='pending' AND j.status='running' THEN 'running' ELSE d.status END"
	}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT d.id,d.execution_id,d.target_json,d.refresh,`+status+`,d.error_code,d.error_message,d.published,d.digest FROM cover_deliveries d JOIN cover_downloads j ON j.id=d.job_id WHERE `+where+` ORDER BY d.rowid`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []coverdelivery.Destination{}
	for rows.Next() {
		var d coverdelivery.Destination
		var raw string
		if err = rows.Scan(
			&d.ID,
			&d.ExecutionID,
			&raw,
			&d.Refresh,
			&d.Status,
			&d.ErrorCode,
			&d.ErrorMessage,
			&d.Published,
			&d.Digest,
		); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &d.Target); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (r *Repository) SaveCoverDestination(ctx context.Context, d coverdelivery.Destination) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE cover_deliveries SET status=?,error_code=?,error_message=?,published=?,digest=? WHERE id=? AND status='pending'`,
		d.Status,
		d.ErrorCode,
		d.ErrorMessage,
		d.Published,
		d.Digest,
		d.ID,
	)
	return err
}
func (r *Repository) CoverCurrent(ctx context.Context, j coverdelivery.Job) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cover_candidates c WHERE c.work_code=? AND c.version=? AND EXISTS(SELECT 1 FROM cover_deliveries WHERE job_id=? AND status='pending')`, j.WorkCode, j.Version, j.ID).
		Scan(&count)
	return count > 0, err
}
func (r *Repository) FinishCover(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE cover_downloads SET
 status=CASE WHEN EXISTS(SELECT 1 FROM cover_deliveries d JOIN cover_candidates c ON c.work_code=cover_downloads.work_code AND c.version=cover_downloads.version WHERE d.job_id=cover_downloads.id AND d.status='pending') THEN 'queued' ELSE 'done' END,
 stage_path=CASE WHEN EXISTS(SELECT 1 FROM cover_deliveries WHERE job_id=cover_downloads.id AND status='pending') THEN stage_path ELSE '' END WHERE id=?`, id)
	return err
}
func (r *Repository) ActiveCoverJobs(ctx context.Context) (map[string]bool, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT DISTINCT j.id FROM cover_downloads j JOIN cover_deliveries d ON d.job_id=j.id JOIN cover_candidates c ON c.work_code=j.work_code AND c.version=j.version WHERE d.status='pending'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
func (r *Repository) RecoverCovers(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE cover_downloads SET status='queued' WHERE status='running'`)
	return err
}
func (r *Repository) CoverReport(ctx context.Context, id string) (coverdelivery.Report, error) {
	items, err := r.coverDestinations(ctx, `d.execution_id=?`, id, true)
	out := coverdelivery.Report{
		Items:  items,
		Counts: map[string]int{"pending": 0, "running": 0, "succeeded": 0, "skipped": 0, "failed": 0, "canceled": 0},
	}
	for _, item := range items {
		out.Counts[item.Status]++
	}
	if err != nil {
		return out, err
	}
	out.Downloads = map[string]int{"total": 0, "queued": 0, "running": 0, "done": 0}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT j.status,COUNT(*) FROM cover_downloads j WHERE EXISTS(SELECT 1 FROM cover_deliveries d WHERE d.job_id=j.id AND d.execution_id=?) GROUP BY j.status`,
		id,
	)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err = rows.Scan(&status, &count); err != nil {
			return out, err
		}
		out.Downloads[status] = count
		out.Downloads["total"] += count
	}
	return out, rows.Err()
}
func (r *Repository) ChangeCoverRequests(ctx context.Context, id string, retry bool) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if retry {
		_, err = tx.ExecContext(
			ctx,
			`UPDATE cover_deliveries SET status='pending',error_code='',error_message='' WHERE execution_id=? AND status IN('failed','canceled') AND job_id IN(SELECT j.id FROM cover_downloads j JOIN cover_candidates c ON c.work_code=j.work_code AND c.version=j.version)`,
			id,
		)
		if err == nil {
			_, err = tx.ExecContext(
				ctx,
				`UPDATE cover_downloads SET status='queued' WHERE status<>'running' AND id IN(SELECT job_id FROM cover_deliveries WHERE execution_id=? AND status='pending')`,
				id,
			)
		}
	} else {
		_, err = tx.ExecContext(
			ctx,
			`UPDATE cover_deliveries SET status='canceled',error_code='CANCELED' WHERE execution_id=? AND status='pending'`,
			id,
		)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) StoredCover(ctx context.Context, id string) (*coverdelivery.Stored, error) {
	var c coverdelivery.Stored
	err := r.db.QueryRowContext(ctx, `SELECT url,name,digest,etag,last_modified FROM cover_files WHERE copy_id=?`, id).
		Scan(&c.URL, &c.Name, &c.Digest, &c.ETag, &c.LastModified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &c, err
}
func (r *Repository) SaveStoredCover(ctx context.Context, id string, c coverdelivery.Stored) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO cover_files VALUES(?,?,?,?,?,?) ON CONFLICT(copy_id) DO UPDATE SET url=excluded.url,name=excluded.name,digest=excluded.digest,etag=excluded.etag,last_modified=excluded.last_modified`,
		id,
		c.URL,
		c.Name,
		c.Digest,
		c.ETag,
		c.LastModified,
	)
	return err
}

var _ coverdelivery.Store = (*Repository)(nil)
