package sqlite

import (
	"database/sql"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Clementine33/preen/backend/internal/librarypath"
	"github.com/Clementine33/preen/backend/internal/works"
)

// WorkCopyRoots reads the copies of one work joined to their library roots
// (ADR 0013). A collected work remains found after all copies disappear.
func (r *Repository) WorkCopyRoots(id works.WorkCode) ([]works.CopyRoot, bool, error) {
	var one int
	switch err := r.db.QueryRow("SELECT 1 FROM works WHERE id = ?", string(id)).Scan(&one); {
	case errors.Is(err, sql.ErrNoRows):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	}

	rows, err := r.db.Query(`
		SELECT wc.id, wc.library_id, wc.rel_path, l.root_path
		FROM work_copies wc
		JOIN libraries l ON l.id = wc.library_id
		WHERE wc.work_id = ?
		ORDER BY wc.rel_path
	`, string(id))
	if err != nil {
		return nil, true, err
	}
	defer rows.Close()

	var out []works.CopyRoot
	for rows.Next() {
		var root works.CopyRoot
		if scanErr := rows.Scan(&root.CopyID, &root.LibraryID, &root.RelPath, &root.RootPath); scanErr != nil {
			return nil, true, scanErr
		}
		out = append(out, root)
	}
	return out, true, rows.Err()
}

// WorkAt reads the work bound to one library directory.
func (r *Repository) WorkAt(libraryID, relPath string) (works.WorkCode, bool, error) {
	var id string
	err := r.db.QueryRow(
		"SELECT work_id FROM work_copies WHERE library_id = ? AND rel_path = ?", libraryID, relPath,
	).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	return works.WorkCode(id), true, nil
}

// exampleDirLimit caps the sample of unclear directory names a scan reports.
const exampleDirLimit = 20

// bindingOutcome is what one reconciliation classified, in the scanner's own
// vocabulary.
type bindingOutcome struct {
	multiple  int
	examples  []string
	truncated bool
}

// syncWorkCopies reconciles the library's bindings against the directories the
// merge just left behind. It runs inside the merge transaction, so the binding
// state and the inventory it is derived from become visible together: a scan
// can never leave the inventory updated and the bindings stale.
//
// The directory name is the authority on which work a directory holds, so a
// directory that names no work - or names several - is unbound rather than
// guessed at, and stays ordinary inventory.
func syncWorkCopies(tx *sql.Tx, libraryID, rootPath string) (bindingOutcome, error) {
	var outcome bindingOutcome
	if libraryID == "" {
		return outcome, nil
	}
	// Match the canonical stored root, preserving drive and UNC root forms.
	rootPath = librarypath.CleanRootPath(rootPath)

	rows, err := tx.Query(`
		SELECT path, name, is_work_root
		FROM entries
		WHERE is_dir = 1 AND root_path = ?
		ORDER BY path
	`, rootPath)
	if err != nil {
		return outcome, err
	}
	type childDir struct {
		path, name string
		isWorkRoot bool
	}
	var children []childDir
	for rows.Next() {
		var child childDir
		if err := rows.Scan(&child.path, &child.name, &child.isWorkRoot); err != nil {
			rows.Close()
			return outcome, err
		}
		children = append(children, child)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return outcome, err
	}
	rows.Close()

	prefix := rootPath
	if prefix != "/" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	// Bound holds the directories that must end up bound. Everything else of
	// this library is unbound below, which is also what removes the binding of a
	// directory that no longer exists.
	bound := map[string]works.WorkCode{}
	workRoots := map[string]bool{}
	for _, child := range children {
		relPath := strings.TrimPrefix(child.path, prefix)
		if librarypath.IsRecoveryRelPath(rootPath, relPath) || !child.isWorkRoot {
			continue
		}
		extraction := works.CodesInDirName(child.name)
		if extraction.Kind == works.DirCodeSingle {
			bound[relPath] = extraction.Code
			workRoots[child.path] = true
		}
	}
	for _, child := range children {
		relPath := strings.TrimPrefix(child.path, prefix)
		if librarypath.IsRecoveryRelPath(rootPath, relPath) || child.isWorkRoot ||
			works.CodesInDirName(child.name).Kind != works.DirCodeMultiple {
			continue
		}
		insideWork := false
		for parent := path.Dir(child.path); parent != rootPath && parent != "." && parent != "/"; parent = path.Dir(parent) {
			if workRoots[parent] {
				insideWork = true
				break
			}
		}
		if !insideWork {
			outcome.multiple++
			outcome.addExample(relPath)
		}
	}

	if err := unbindMissing(tx, libraryID, bound); err != nil {
		return outcome, err
	}
	if err := bindDirectories(tx, libraryID, bound); err != nil {
		return outcome, err
	}
	return outcome, nil
}

// addExample records one unclear directory name, up to the report's cap.
func (o *bindingOutcome) addExample(name string) {
	if len(o.examples) >= exampleDirLimit {
		o.truncated = true
		return
	}
	o.examples = append(o.examples, name)
}

// unbindMissing drops every binding of this library that the reconciliation did
// not keep.
func unbindMissing(tx *sql.Tx, libraryID string, bound map[string]works.WorkCode) error {
	if len(bound) == 0 {
		_, err := tx.Exec("DELETE FROM work_copies WHERE library_id = ?", libraryID)
		return err
	}
	// Keep the Go classification authoritative without one SQL parameter per work.
	// The temporary table belongs to this transaction; rollback also removes its DDL.
	if _, err := tx.Exec(
		`CREATE TEMP TABLE scan_bound_copy_paths (rel_path TEXT PRIMARY KEY) WITHOUT ROWID`,
	); err != nil {
		return err
	}
	paths := make([]any, 0, len(bound))
	for relPath := range bound {
		paths = append(paths, relPath)
	}
	for start := 0; start < len(paths); start += defaultBatchSize {
		batch := paths[start:min(start+defaultBatchSize, len(paths))]
		values := strings.TrimSuffix(strings.Repeat("(?),", len(batch)), ",")
		if _, err := tx.Exec("INSERT INTO temp.scan_bound_copy_paths (rel_path) VALUES "+values, batch...); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`
		DELETE FROM work_copies WHERE library_id = ?
		AND rel_path NOT IN (SELECT rel_path FROM temp.scan_bound_copy_paths)
	`, libraryID); err != nil {
		return err
	}
	_, err := tx.Exec("DROP TABLE temp.scan_bound_copy_paths")
	return err
}

// bindDirectories reconciles the works and copies discovered in this scan.
// Existing collection times and copy identities survive rescans and changed bindings.
func bindDirectories(tx *sql.Tx, libraryID string, bound map[string]works.WorkCode) error {
	workStmt, err := tx.Prepare(`
		INSERT INTO works (id, collected_at, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer workStmt.Close()
	copyStmt, err := tx.Prepare(`
		INSERT INTO work_copies (id, work_id, library_id, rel_path, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(library_id, rel_path) DO UPDATE SET work_id = excluded.work_id
		WHERE work_copies.work_id IS NOT excluded.work_id
	`)
	if err != nil {
		return err
	}
	defer copyStmt.Close()
	now := formatWorksTimestamp(time.Now())
	var added []works.WorkCode
	for relPath, workID := range bound {
		result, err := workStmt.Exec(string(workID), now, now, now)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count > 0 {
			added = append(added, workID)
		}
		if _, err := copyStmt.Exec(uuid.NewString(), string(workID), libraryID, relPath, now); err != nil {
			return err
		}
	}
	if len(added) > 0 {
		evidence, err := loadEvidence(tx)
		if err != nil {
			return err
		}
		return recomputeRelations(tx, works.Resolve(evidence), added)
	}
	return nil
}

// unbindLibraryCopies drops every binding of one library, which a root change
// and a library deletion both need: the bindings describe directories under the
// old root, and they are meaningless without it.
func unbindLibraryCopies(tx *sql.Tx, libraryID string) error {
	_, err := tx.Exec("DELETE FROM work_copies WHERE library_id = ?", libraryID)
	return err
}
