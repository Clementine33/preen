package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/onsei/organizer/backend/internal/librarypath"
)

// RelocateDirectory rebases all mutable path identities in one transaction.
// Repeating a completed relocation updates no rows. Target conflicts roll back
// every change; frozen plans and requests remain historical.
func (r *Repository) RelocateDirectory(ctx context.Context, root, source, target string) error {
	root = librarypath.CleanRootPath(root)
	source, target = librarypath.CleanRootPath(source), librarypath.CleanRootPath(target)
	if source == target || !librarypath.IsWithinRoot(root, source) || !librarypath.IsWithinRoot(root, target) ||
		source == root ||
		target == root ||
		filepath.Dir(source) != filepath.Dir(target) {
		return fmt.Errorf("invalid directory relocation")
	}
	sourceRel, err := filepath.Rel(filepath.FromSlash(root), filepath.FromSlash(source))
	if err != nil {
		return err
	}
	targetRel, err := filepath.Rel(filepath.FromSlash(root), filepath.FromSlash(target))
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := relocateEntries(ctx, tx, source, target); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE generation_records SET path=? || substr(path,length(?)+1) WHERE path=? OR substr(path,1,length(?)+1)=? || '/'`,
		target,
		source,
		source,
		source,
		source,
	); err != nil {
		return fmt.Errorf("relocate credentials: %w", err)
	}
	if err := relocateCopies(
		ctx,
		tx,
		librarypath.RootPathKey(root),
		filepath.ToSlash(sourceRel),
		filepath.ToSlash(targetRel),
	); err != nil {
		return err
	}
	if err := relocateMembers(
		ctx,
		tx,
		librarypath.RootPathKey(root),
		source,
		target,
		filepath.ToSlash(sourceRel),
		filepath.ToSlash(targetRel),
	); err != nil {
		return err
	}
	return tx.Commit()
}

func relocateEntries(ctx context.Context, tx *sql.Tx, source, target string) error {
	_, err := tx.ExecContext(ctx, `UPDATE entries SET
 path=? || substr(path,length(?)+1),
 root_path=CASE WHEN root_path=? OR substr(root_path,1,length(?)+1)=? || '/' THEN ? || substr(root_path,length(?)+1) ELSE root_path END,
 parent_path=CASE WHEN parent_path=? OR substr(parent_path,1,length(?)+1)=? || '/' THEN ? || substr(parent_path,length(?)+1) ELSE parent_path END,
 name=CASE WHEN path=? THEN ? ELSE name END,
 path_posix=CASE WHEN path_posix=? OR substr(path_posix,1,length(?)+1)=? || '/' THEN ? || substr(path_posix,length(?)+1) ELSE path_posix END
 WHERE path=? OR substr(path,1,length(?)+1)=? || '/'`,
		target, source,
		source, source, source, target, source,
		source, source, source, target, source,
		source, filepath.Base(target),
		source, source, source, target, source,
		source, source, source)
	if err != nil {
		return fmt.Errorf("relocate inventory: %w", err)
	}
	return nil
}
func relocateCopies(ctx context.Context, tx *sql.Tx, root, source, target string) error {
	_, err := tx.ExecContext(ctx, `UPDATE work_copies SET rel_path=? || substr(rel_path,length(?)+1)
 WHERE library_id IN (SELECT id FROM libraries WHERE root_path_key=?)
 AND (rel_path=? OR substr(rel_path,1,length(?)+1)=? || '/')`, target, source, root, source, source, source)
	if err != nil {
		return fmt.Errorf("relocate copies: %w", err)
	}
	return nil
}
func relocateMembers(ctx context.Context, tx *sql.Tx, root, source, target, sourceRel, targetRel string) error {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE workset_members SET
 rel_path=? || substr(rel_path,length(?)+1),
 folder_path=CASE WHEN folder_path=? OR substr(folder_path,1,length(?)+1)=? || '/' THEN ? || substr(folder_path,length(?)+1) ELSE folder_path END,
 folder_name=CASE WHEN folder_path=? THEN ? ELSE folder_name END
 WHERE workset_id IN (SELECT id FROM worksets WHERE root_path_key=?)
 AND (rel_path=? OR substr(rel_path,1,length(?)+1)=? || '/')`,
		targetRel,
		sourceRel,
		source,
		source,
		source,
		target,
		source,
		source,
		filepath.Base(target),
		root,
		sourceRel,
		sourceRel,
		sourceRel,
	)
	if err != nil {
		return fmt.Errorf("relocate members: %w", err)
	}
	return nil
}
