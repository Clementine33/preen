package sqlite

import (
	"database/sql"
	"errors"

	"github.com/Clementine33/preen/backend/internal/inventory"

	"github.com/Clementine33/preen/backend/internal/library"
)

func checkLibraryRootOverlap(db interface {
	Query(string, ...any) (*sql.Rows, error)
}, root, exceptID string) error {
	rows, err := db.Query("SELECT root_path FROM libraries WHERE id <> ?", exceptID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var other string
		if err := rows.Scan(&other); err != nil {
			return err
		}
		if err := library.CheckRootOverlap(root, other); err != nil {
			return err
		}
	}
	return rows.Err()
}

// CheckScanRoot refuses an affected legacy configuration before traversing media.
func (r *Repository) CheckScanRoot(root string) error {
	return checkScanRoot(r.db, root)
}

func checkScanRoot(db interface {
	Query(string, ...any) (*sql.Rows, error)
}, root string) error {
	err := checkLibraryRootOverlap(db, root, "")
	if errors.Is(err, library.ErrLibraryRootOverlap) {
		return inventory.NewError(inventory.ErrKindConflict, "LIBRARY_ROOT_OVERLAP", err.Error(), err)
	}
	if err != nil {
		return inventory.NewError(inventory.ErrKindInternal, "INTERNAL", "failed to check library roots", err)
	}
	return nil
}
