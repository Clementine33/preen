package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Clementine33/preen/backend/internal/collections"
	"github.com/Clementine33/preen/backend/internal/works"
)

func (r *Repository) RemoveCollectionWork(ctx context.Context, id string, work works.WorkCode, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, lookupErr := collectionInTransaction(ctx, tx, id); lookupErr != nil {
		return lookupErr
	}
	if err := removeCollectionWork(ctx, tx, id, work, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if id == collections.FavoritesID {
		r.catalogChanged()
	}
	return nil
}

func removeCollectionWork(ctx context.Context, tx *sql.Tx, id string, work works.WorkCode, now time.Time) error {
	result, err := tx.ExecContext(
		ctx,
		"DELETE FROM collection_works WHERE collection_id = ? AND work_id = ?",
		id,
		string(work),
	)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return err
	}
	return touchCollection(ctx, tx, id, now)
}

func (r *Repository) MoveCollectionWork(
	ctx context.Context, id string, work works.WorkCode, before *works.WorkCode, now time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, lookupErr := collectionInTransaction(ctx, tx, id); lookupErr != nil {
		return lookupErr
	}
	from, err := collectionPosition(ctx, tx, id, work)
	if err != nil {
		return err
	}
	var to int64
	if before == nil {
		err = tx.QueryRowContext(ctx, "SELECT MAX(position) FROM collection_works WHERE collection_id = ?", id).
			Scan(&to)
	} else {
		to, err = collectionPosition(ctx, tx, id, *before)
		if from < to {
			to--
		}
	}
	if err != nil {
		return err
	}
	if from == to {
		return nil
	}
	if err := shiftCollectionRange(ctx, tx, id, from, to); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE collection_works SET position = ? WHERE collection_id = ? AND work_id = ?",
		to, id, string(work)); err != nil {
		return err
	}
	if err := touchCollection(ctx, tx, id, now); err != nil {
		return err
	}
	return tx.Commit()
}

func collectionPosition(ctx context.Context, tx *sql.Tx, id string, work works.WorkCode) (int64, error) {
	var position int64
	err := tx.QueryRowContext(ctx, "SELECT position FROM collection_works WHERE collection_id = ? AND work_id = ?",
		id, string(work)).Scan(&position)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, works.NewError(
			works.KindConflict,
			collections.CodeMemberConflict,
			"collection member no longer exists",
		)
	}
	return position, err
}

func shiftCollectionRange(ctx context.Context, tx *sql.Tx, id string, from, to int64) error {
	if from < to {
		_, err := tx.ExecContext(ctx, `UPDATE collection_works SET position = position - 1
		 WHERE collection_id = ? AND position > ? AND position <= ?`, id, from, to)
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE collection_works SET position = position + 1
	 WHERE collection_id = ? AND position >= ? AND position < ?`, id, to, from)
	return err
}
