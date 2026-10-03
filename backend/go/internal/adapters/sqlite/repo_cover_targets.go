package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/onsei/organizer/backend/internal/librarypath"
	"github.com/onsei/organizer/backend/internal/works/coverdelivery"
)

func (r *Repository) CoverTargets(
	ctx context.Context,
	root string,
	folders []string,
) (map[string]coverdelivery.Target, error) {
	out := make(map[string]coverdelivery.Target, len(folders))
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT wc.id,wc.work_id,wc.rel_path,l.root_path FROM work_copies wc JOIN libraries l ON l.id=wc.library_id WHERE l.root_path_key=?`,
		librarypath.RootPathKey(root),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	wanted := map[string]string{}
	for _, f := range folders {
		wanted[librarypath.RootPathKey(f)] = f
	}
	for rows.Next() {
		var t coverdelivery.Target
		if err = rows.Scan(&t.CopyID, &t.WorkCode, &t.RelPath, &t.LibraryRoot); err != nil {
			return nil, err
		}
		folder := librarypath.JoinRel(t.LibraryRoot, t.RelPath)
		if selected, ok := wanted[librarypath.RootPathKey(folder)]; ok {
			out[selected] = t
		}
	}
	return out, rows.Err()
}
func (r *Repository) ResolveCoverCopy(ctx context.Context, id string) (coverdelivery.Target, error) {
	var t coverdelivery.Target
	err := r.db.QueryRowContext(ctx, `SELECT wc.id,wc.work_id,wc.rel_path,l.root_path FROM work_copies wc JOIN libraries l ON l.id=wc.library_id WHERE wc.id=?`, id).
		Scan(&t.CopyID, &t.WorkCode, &t.RelPath, &t.LibraryRoot)
	if errors.Is(err, sql.ErrNoRows) {
		return t, coverdelivery.Failure("COPY_NOT_FOUND", err)
	}
	return t, err
}
