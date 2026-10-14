package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/onsei/organizer/backend/internal/collections"
	"github.com/onsei/organizer/backend/internal/works"
)

const collectionProjection = `SELECT c.id, c.name,
 (SELECT COUNT(*) FROM collection_works m WHERE m.collection_id = c.id), c.created_at, c.updated_at
 FROM collections c`

func (r *Repository) ListCollections(ctx context.Context, window collections.Window) (collections.Page, error) {
	page := collections.Page{Collections: []collections.Collection{}}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return page, err
	}
	defer tx.Rollback()
	if countErr := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM collections").Scan(&page.Total); countErr != nil {
		return page, countErr
	}
	rows, err := tx.QueryContext(ctx, collectionProjection+
		" ORDER BY (c.id = 'favorites') DESC, c.created_at, c.id LIMIT ? OFFSET ?", window.Limit, window.Offset())
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var c collections.Collection
		if err := rows.Scan(&c.ID, &c.Name, &c.WorkCount, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return page, err
		}
		page.Collections = append(page.Collections, c)
	}
	return page, rows.Err()
}

func collectionInTransaction(ctx context.Context, tx *sql.Tx, id string) (collections.Collection, error) {
	var c collections.Collection
	err := tx.QueryRowContext(ctx, collectionProjection+" WHERE c.id = ?", id).
		Scan(&c.ID, &c.Name, &c.WorkCount, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, works.NotFound(collections.CodeNotFound, "no such collection")
	}
	return c, err
}

func (r *Repository) GetCollection(
	ctx context.Context,
	id string,
	window collections.Window,
) (collections.Detail, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return collections.Detail{}, err
	}
	defer tx.Rollback()
	c, err := collectionInTransaction(ctx, tx, id)
	if err != nil {
		return collections.Detail{}, err
	}
	order := ""
	if window.Direction == works.Descending {
		order = " DESC"
	}
	rows, err := tx.QueryContext(ctx, listProjection+`
	 AND w.id IN (SELECT work_id FROM collection_works WHERE collection_id = ?
	 ORDER BY position`+order+`, work_id`+order+` LIMIT ? OFFSET ?)
	 ORDER BY (SELECT position FROM collection_works m WHERE m.collection_id = ? AND m.work_id = w.id)`+order+
		`, w.id`+order,
		id, window.Limit, window.Offset(), id)
	if err != nil {
		return collections.Detail{}, err
	}
	summaries, err := r.readWorkSummaries(rows)
	return collections.Detail{Collection: c, Works: summaries, Total: c.WorkCount}, err
}

func (r *Repository) CreateCollection(ctx context.Context, name string, now time.Time) (collections.Collection, error) {
	c := collections.Collection{ID: uuid.NewString(), Name: name,
		CreatedAt: formatWorksTimestamp(now), UpdatedAt: formatWorksTimestamp(now)}
	_, err := r.db.ExecContext(ctx, "INSERT INTO collections(id,name,created_at,updated_at) VALUES(?,?,?,?)",
		c.ID, c.Name, c.CreatedAt, c.UpdatedAt)
	return c, err
}

func (r *Repository) RenameCollection(
	ctx context.Context,
	id, name string,
	now time.Time,
) (collections.Collection, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return collections.Collection{}, err
	}
	defer tx.Rollback()
	c, err := collectionInTransaction(ctx, tx, id)
	if err != nil {
		return c, err
	}
	c.Name, c.UpdatedAt = name, formatWorksTimestamp(now)
	if _, err := tx.ExecContext(
		ctx,
		"UPDATE collections SET name = ?, updated_at = ? WHERE id = ?",
		c.Name,
		c.UpdatedAt,
		id,
	); err != nil {
		return c, err
	}
	return c, tx.Commit()
}

func (r *Repository) DeleteCollection(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM collections WHERE id = ?", id)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return works.NotFound(collections.CodeNotFound, "no such collection")
	}
	return nil
}

func collectionBlockers(tx *sql.Tx, work works.WorkCode) ([]string, error) {
	rows, err := tx.Query(`SELECT c.id,c.name FROM collection_works m JOIN collections c ON c.id = m.collection_id
	 WHERE m.work_id = ? ORDER BY c.id`, string(work))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var blockers []string
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		blockers = append(blockers, "a collection holds this work: "+id+" "+name)
	}
	return blockers, rows.Err()
}
