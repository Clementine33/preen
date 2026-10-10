package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/onsei/organizer/backend/internal/librarypath"
	"github.com/onsei/organizer/backend/internal/works"
)

// RelocateDirectory rebases all mutable path identities in one transaction.
// Repeating a completed relocation updates no rows. Target conflicts roll back
// every change; frozen plans and requests remain historical.
func (r *Repository) RelocateDirectory(ctx context.Context, root, source, target string) error {
	return r.RelocateDirectoryAs(ctx, root, source, target, "")
}

// RelocateDirectoryAs is RelocateDirectory that also rebinds the moved copy to
// the work named by code, in the same transaction. A rename whose new name
// carries another work code must not leave the copy bound to the old one; an
// empty code keeps the binding as it is. The source may equal the target (a
// folder renamed by hand and then renamed back): no path moves, only the
// rebind applies.
//
// Every path match is a binary range, never substr(): SQLite can search a
// range on the primary key but must scan the table for substr(), and a
// whole-library rename relocates once per folder.
func (r *Repository) RelocateDirectoryAs(ctx context.Context, root, source, target, code string) error {
	root = librarypath.CleanRootPath(root)
	source, target = librarypath.CleanRootPath(source), librarypath.CleanRootPath(target)
	if !librarypath.IsWithinRoot(root, source) || !librarypath.IsWithinRoot(root, target) ||
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
	sourceRel, targetRel = filepath.ToSlash(sourceRel), filepath.ToSlash(targetRel)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rootKey := librarypath.RootPathKey(root)
	libraryID, err := optionalID(ctx, tx, `SELECT id FROM libraries WHERE root_path_key=?`, rootKey)
	if err != nil {
		return fmt.Errorf("relocate: %w", err)
	}
	if source != target {
		if err := relocateEntries(ctx, tx, source, target); err != nil {
			return err
		}
		if err := relocateCredentials(ctx, tx, source, target); err != nil {
			return err
		}
		if err := relocateCopies(ctx, tx, libraryID, sourceRel, targetRel); err != nil {
			return err
		}
		if err := relocateMembers(ctx, tx, rootKey, source, target, sourceRel, targetRel); err != nil {
			return err
		}
	}
	if code != "" && libraryID != "" {
		if err := rebindCopy(tx, libraryID, targetRel, code); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// subtree is the binary range holding everything below p: "/" is followed by
// "0" in byte order, so [p+"/", p+"0") spans exactly the descendants.
func subtree(p string) (from, to string) { return p + "/", p + "0" }

// optionalID reads one id, or "" when no row matches.
func optionalID(ctx context.Context, tx *sql.Tx, query string, args ...any) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, query, args...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// rebindCopy points the copy at rel_path to the work with the given code, the
// way a scan would bind the renamed directory: the copy keeps its identity and
// the work is created as a placeholder when the library has never seen it.
func rebindCopy(tx *sql.Tx, libraryID, relPath, code string) error {
	if err := bindDirectories(tx, libraryID, map[string]works.WorkCode{relPath: works.WorkCode(code)}); err != nil {
		return fmt.Errorf("rebind copy: %w", err)
	}
	return nil
}

// The statements whose WHERE must stay a range search; the tests EXPLAIN them.
const (
	relocateEntriesSQL = `UPDATE entries SET
 path=? || substr(path,length(?)+1),
 root_path=CASE WHEN root_path=? OR substr(root_path,1,length(?)+1)=? || '/' THEN ? || substr(root_path,length(?)+1) ELSE root_path END,
 parent_path=CASE WHEN parent_path=? OR substr(parent_path,1,length(?)+1)=? || '/' THEN ? || substr(parent_path,length(?)+1) ELSE parent_path END,
 name=CASE WHEN path=? THEN ? ELSE name END,
 path_posix=CASE WHEN path_posix=? OR substr(path_posix,1,length(?)+1)=? || '/' THEN ? || substr(path_posix,length(?)+1) ELSE path_posix END
 WHERE path=? OR (path>=? AND path<?)`
	relocateCredentialsSQL = `UPDATE generation_records SET path=? || substr(path,length(?)+1) WHERE path=? OR (path>=? AND path<?)`
	selectCopiesSQL        = `SELECT id, rel_path FROM work_copies WHERE library_id=? AND (rel_path=? OR (rel_path>=? AND rel_path<?))`
	selectMembersSQL       = `SELECT member_index, rel_path, folder_path, folder_name FROM workset_members
 WHERE workset_id=? AND (rel_path=? OR (rel_path>=? AND rel_path<?))`
)

func relocateEntries(ctx context.Context, tx *sql.Tx, source, target string) error {
	from, to := subtree(source)
	_, err := tx.ExecContext(ctx, relocateEntriesSQL,
		target, source,
		source, source, source, target, source,
		source, source, source, target, source,
		source, filepath.Base(target),
		source, source, source, target, source,
		source, from, to)
	if err != nil {
		return fmt.Errorf("relocate inventory: %w", err)
	}
	return nil
}

func relocateCredentials(ctx context.Context, tx *sql.Tx, source, target string) error {
	from, to := subtree(source)
	_, err := tx.ExecContext(ctx, relocateCredentialsSQL, target, source, source, from, to)
	if err != nil {
		return fmt.Errorf("relocate credentials: %w", err)
	}
	return nil
}

// relocateCopies moves the copies at or below the source. The matching rows
// are read first: an UPDATE of an indexed column is planned on the library
// alone, while the SELECT searches (library_id, rel_path) by range.
func relocateCopies(ctx context.Context, tx *sql.Tx, libraryID, source, target string) error {
	if libraryID == "" {
		return nil
	}
	from, to := subtree(source)
	rows, err := tx.QueryContext(ctx, selectCopiesSQL, libraryID, source, from, to)
	if err != nil {
		return fmt.Errorf("relocate copies: %w", err)
	}
	type moved struct{ id, relPath string }
	var found []moved
	for rows.Next() {
		var m moved
		if err := rows.Scan(&m.id, &m.relPath); err != nil {
			rows.Close()
			return fmt.Errorf("relocate copies: %w", err)
		}
		found = append(found, m)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("relocate copies: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("relocate copies: %w", err)
	}
	for _, m := range found {
		if _, err := tx.ExecContext(
			ctx, `UPDATE work_copies SET rel_path=? WHERE id=?`, target+m.relPath[len(source):], m.id,
		); err != nil {
			return fmt.Errorf("relocate copies: %w", err)
		}
	}
	return nil
}

// relocateMembers moves the workset members at or below the source, reading
// them by range first for the same reason as relocateCopies.
func relocateMembers(ctx context.Context, tx *sql.Tx, rootKey, source, target, sourceRel, targetRel string) error {
	ids, err := tx.QueryContext(ctx, `SELECT id FROM worksets WHERE root_path_key=?`, rootKey)
	if err != nil {
		return fmt.Errorf("relocate members: %w", err)
	}
	var worksets []string
	for ids.Next() {
		var id string
		if err := ids.Scan(&id); err != nil {
			ids.Close()
			return fmt.Errorf("relocate members: %w", err)
		}
		worksets = append(worksets, id)
	}
	if err := ids.Close(); err != nil {
		return fmt.Errorf("relocate members: %w", err)
	}
	if err := ids.Err(); err != nil {
		return fmt.Errorf("relocate members: %w", err)
	}
	from, to := subtree(sourceRel)
	for _, worksetID := range worksets {
		rows, err := tx.QueryContext(ctx, selectMembersSQL, worksetID, sourceRel, from, to)
		if err != nil {
			return fmt.Errorf("relocate members: %w", err)
		}
		type member struct {
			index                         int
			relPath, folderPath, folderNm string
		}
		var found []member
		for rows.Next() {
			var m member
			if err := rows.Scan(&m.index, &m.relPath, &m.folderPath, &m.folderNm); err != nil {
				rows.Close()
				return fmt.Errorf("relocate members: %w", err)
			}
			found = append(found, m)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("relocate members: %w", err)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("relocate members: %w", err)
		}
		for _, m := range found {
			folderPath, folderName := m.folderPath, m.folderNm
			if m.folderPath == source {
				folderPath, folderName = target, filepath.Base(target)
			} else if strings.HasPrefix(m.folderPath, source+"/") {
				folderPath = target + m.folderPath[len(source):]
			}
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE workset_members SET rel_path=?, folder_path=?, folder_name=? WHERE workset_id=? AND member_index=?`,
				targetRel+m.relPath[len(sourceRel):], folderPath, folderName, worksetID, m.index,
			); err != nil {
				return fmt.Errorf("relocate members: %w", err)
			}
		}
	}
	return nil
}
