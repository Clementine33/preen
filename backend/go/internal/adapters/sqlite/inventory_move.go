package sqlite

import (
	"context"
	"fmt"

	"github.com/Clementine33/preen/backend/internal/librarypath"
)

// MoveInventoryPath synchronizes a successful direct member write. An empty
// target removes the source: recovery directories never enter the inventory.
// Directory moves keep file rows and their measured facts at stable references.
func (r *Repository) MoveInventoryPath(ctx context.Context, root, source, target string) error {
	root, source = librarypath.CleanRootPath(root), librarypath.CleanRootPath(source)
	if target != "" {
		target = librarypath.CleanRootPath(target)
	}
	if source == root || !librarypath.IsWithinRoot(root, source) ||
		(target != "" && (target == root || !librarypath.IsWithinRoot(root, target))) {
		return fmt.Errorf("invalid inventory move")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	parent, name := splitInventoryPath(source)
	if target == "" {
		from, to := subtree(source)
		if _, err = tx.ExecContext(
			ctx,
			`DELETE FROM inventory_dirs WHERE path=? OR (path>=? AND path<?)`,
			source,
			from,
			to,
		); err != nil {
			return err
		}
		if _, err = tx.ExecContext(
			ctx,
			`DELETE FROM inventory_files WHERE dir_ref=(SELECT id FROM inventory_dirs WHERE path=?) AND name=?`,
			parent,
			name,
		); err != nil {
			return err
		}
		if _, err = tx.ExecContext(
			ctx,
			`DELETE FROM generation_records WHERE path=? OR (path>=? AND path<?)`,
			source,
			from,
			to,
		); err != nil {
			return err
		}
	} else {
		var isDir bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_dirs WHERE path=?)`, source).
			Scan(&isDir); err != nil {
			return err
		}
		if isDir {
			if err = relocateEntries(ctx, tx, source, target); err != nil {
				return err
			}
			if err = relocateCredentials(ctx, tx, source, target); err != nil {
				return err
			}
		} else {
			dir, newName := splitInventoryPath(target)
			if parent != dir && name != newName {
				return fmt.Errorf("inventory move changes both directory and filename")
			}
			if err = ensureInventoryDirectory(tx, dir, root); err != nil {
				return err
			}
			if parent == dir {
				_, err = tx.ExecContext(
					ctx,
					`UPDATE inventory_files SET name=? WHERE dir_ref=(SELECT id FROM inventory_dirs WHERE path=?) AND name=?`,
					newName,
					parent,
					name,
				)
			} else {
				_, err = tx.ExecContext(
					ctx,
					`UPDATE inventory_files SET dir_ref=(SELECT id FROM inventory_dirs WHERE path=?) WHERE dir_ref=(SELECT id FROM inventory_dirs WHERE path=?) AND name=?`,
					dir,
					parent,
					name,
				)
			}
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(
				ctx,
				`UPDATE generation_records SET path=? WHERE path=?`,
				target,
				source,
			); err != nil {
				return err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	r.catalogChanged()
	return nil
}
