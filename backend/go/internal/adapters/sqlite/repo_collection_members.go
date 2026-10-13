package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/onsei/organizer/backend/internal/collections"
	"github.com/onsei/organizer/backend/internal/works"
)

func (r *Repository) AppendCollectionWorks(
	ctx context.Context, id string, ids []works.WorkCode, now time.Time,
) (collections.AppendResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return collections.AppendResult{}, err
	}
	defer tx.Rollback()
	if _, lookupErr := collectionInTransaction(ctx, tx, id); lookupErr != nil {
		return collections.AppendResult{}, lookupErr
	}
	if validationErr := validateCollectionWorks(ctx, tx, ids); validationErr != nil {
		return collections.AppendResult{}, validationErr
	}
	result, err := appendCollectionWorks(ctx, tx, id, ids, now)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	if id == collections.FavoritesID && len(result.Added) > 0 {
		r.catalogChanged()
	}
	return result, nil
}

// validateCollectionWorks reports the entire bad set before any member is written.
func validateCollectionWorks(ctx context.Context, tx *sql.Tx, ids []works.WorkCode) error {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = string(id)
	}
	rows, err := tx.QueryContext(
		ctx,
		"SELECT id, collected_at FROM works WHERE id IN ("+placeholderList(len(ids))+")",
		args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	found := make(map[works.WorkCode]bool, len(ids))
	for rows.Next() {
		var id works.WorkCode
		var stamp sql.NullString
		if err := rows.Scan(&id, &stamp); err != nil {
			return err
		}
		found[id] = stamp.Valid
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var bad []string
	missing := false
	for _, id := range ids {
		eligible, exists := found[id]
		if !eligible {
			bad = append(bad, string(id))
			missing = missing || !exists
		}
	}
	if len(bad) == 0 {
		return nil
	}
	refusal := works.NewError(works.KindConflict, "WORK_IS_PLACEHOLDER", "some works have never been found by a scan")
	if missing {
		refusal = works.NotFound(works.CodeWorkNotFound, "some works do not exist")
	}
	refusal.Details = bad
	return refusal
}

func appendCollectionWorks(
	ctx context.Context, tx *sql.Tx, id string, ids []works.WorkCode, now time.Time,
) (collections.AppendResult, error) {
	result := collections.AppendResult{Added: []works.WorkCode{}, Skipped: []works.WorkCode{}}
	var position int64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COALESCE(MAX(position), -1) + 1 FROM collection_works WHERE collection_id = ?",
		id,
	).Scan(&position); err != nil {
		return result, err
	}
	for _, work := range ids {
		write, err := tx.ExecContext(ctx, `INSERT INTO collection_works(collection_id,work_id,position) VALUES(?,?,?)
		 ON CONFLICT(collection_id,work_id) DO NOTHING`, id, string(work), position)
		if err != nil {
			return result, err
		}
		changed, err := write.RowsAffected()
		if err != nil {
			return result, err
		}
		if changed == 0 {
			result.Skipped = append(result.Skipped, work)
		} else {
			result.Added = append(result.Added, work)
			position++
		}
	}
	if len(result.Added) > 0 {
		if err := touchCollection(ctx, tx, id, now); err != nil {
			return result, err
		}
	}
	err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM collection_works WHERE collection_id = ?", id).
		Scan(&result.WorkCount)
	return result, err
}

func touchCollection(ctx context.Context, tx *sql.Tx, id string, now time.Time) error {
	_, err := tx.ExecContext(ctx, "UPDATE collections SET updated_at = ? WHERE id = ?", formatWorksTimestamp(now), id)
	return err
}
