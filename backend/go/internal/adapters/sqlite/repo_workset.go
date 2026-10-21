package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Clementine33/preen/backend/internal/library"
	"github.com/Clementine33/preen/backend/internal/workset"
)

// ==================== workset.Workset aggregates ====================

// ==================== workset.Workset CRUD ====================

const worksetColumns = `id, title, library_id, root_path, root_path_key, version, creation_idem_key, creation_request_hash, created_at, updated_at`

func scanWorkset(scanner interface{ Scan(...any) error }) (*workset.Workset, error) {
	var w workset.Workset
	var libraryID, creationKey sql.NullString
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&w.ID,
		&w.Title,
		&libraryID,
		&w.RootPath,
		&w.RootPathKey,
		&w.Version,
		&creationKey,
		&w.CreationRequestHash,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	w.LibraryID = libraryID.String
	w.CreationIdemKey = creationKey.String
	w.CreatedAt = parseTimestamp(createdAt)
	w.UpdatedAt = parseTimestamp(updatedAt)
	return &w, nil
}

// GetCurrentWorkset returns the current processing record of one
// library, or nil when it has none.
func (r *Repository) GetCurrentWorkset(libraryID string) (*workset.Workset, error) {
	row := r.db.QueryRow(
		`SELECT `+worksetColumns+` FROM worksets WHERE library_id = ?`,
		libraryID,
	)
	w, err := scanWorkset(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return w, nil
}

// GetWorkset retrieves a workset by id.
func (r *Repository) GetWorkset(id string) (*workset.Workset, error) {
	row := r.db.QueryRow(`SELECT `+worksetColumns+` FROM worksets WHERE id = ?`, id)
	w, err := scanWorkset(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, workset.ErrWorksetNotFound
		}
		return nil, err
	}
	return w, nil
}

// GetWorksetByCreationIdemKey retrieves the workset owned by a creation
// idempotency key. Returns nil when no key match exists.
func (r *Repository) GetWorksetByCreationIdemKey(key string) (*workset.Workset, error) {
	row := r.db.QueryRow(`SELECT `+worksetColumns+` FROM worksets WHERE creation_idem_key = ?`, key)
	w, err := scanWorkset(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return w, nil
}

// ClearExpiredWorksetIdemKey releases an expired creation idempotency key so a
// retried create transparently makes a new workset. The row itself is never
// deleted.
func (r *Repository) ClearExpiredWorksetIdemKey(id string, cutoff time.Time) error {
	_, err := r.db.Exec(`
		UPDATE worksets SET creation_idem_key = NULL
		WHERE id = ? AND julianday(created_at) < julianday(?)
	`, id, cutoff.Format(timeFormat))
	return err
}

// HasActiveSession reports whether any planning session or execution is queued
// or running. It is the database half of the file-management admission
// control: those sessions live on disk in the database and outlive any single
// request, so a writer asks rather than tracks them.
func (r *Repository) HasActiveSession() (bool, error) {
	for _, query := range []string{
		`SELECT COUNT(*) FROM plan_generations WHERE status IN ('queued','running')`,
		`SELECT COUNT(*) FROM plan_executions WHERE status IN ('queued','running')`,
	} {
		var n int
		if err := r.db.QueryRow(query).Scan(&n); err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
	}
	return false, nil
}

// ReplaceCurrentWorkset creates the library's new shared current batch and removes the record it replaces, all in one transaction:
// members, operations, drafts, plan snapshots, revisions and sessions of the
// old record are gone exactly when the new record becomes visible.
//
// expectedCurrentID is the record the caller saw as current ("" when it saw
// none). A mismatch is workset.ErrCurrentRecordChanged. A replaced record holding a
// queued or running session is workset.ErrRecordBusy. A second row for the same
// library is refused by the unique index; a repeated creation key
// is workset.ErrWorksetIdemConflict.
func (r *Repository) ReplaceCurrentWorkset(
	w *workset.Workset,
	members []workset.WorksetMember,
	operations []workset.Operation,
	drafts []workset.OperationDraft,
	expectedCurrentID string,
) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin replace workset tx: %w", err)
	}
	defer tx.Rollback()

	current, err := currentWorksetTx(tx, w.LibraryID)
	if err != nil {
		return err
	}
	currentID := ""
	if current != nil {
		currentID = current.ID
	}
	if currentID != expectedCurrentID {
		return workset.ErrCurrentRecordChanged
	}
	if current != nil {
		busy, err := recordHasActiveSessionTx(tx, current.ID)
		if err != nil {
			return err
		}
		if busy {
			return workset.ErrRecordBusy
		}
		if err := deleteWorksetDataTx(tx, current.ID); err != nil {
			return err
		}
	}

	if err := insertWorkset(tx, w, members); err != nil {
		return err
	}
	for _, operation := range operations {
		if err := insertOperation(tx, operation); err != nil {
			return err
		}
	}
	for _, draft := range drafts {
		if err := upsertOperationDraft(tx, draft); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// currentWorksetTx reads the current record of a library
// inside a transaction; nil when there is none.
func currentWorksetTx(tx *sql.Tx, libraryID string) (*workset.Workset, error) {
	w, err := scanWorkset(tx.QueryRow(
		`SELECT `+worksetColumns+` FROM worksets WHERE library_id = ?`,
		libraryID,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return w, nil
}

// recordHasActiveSessionTx reports whether a record owns a queued or running
// planning session or execution. The check runs inside the same transaction as
// the write it guards: claim/complete updates and this read serialize on the
// same SQLite writer, so a committed replace cannot strand a running session.
func recordHasActiveSessionTx(tx *sql.Tx, worksetID string) (bool, error) {
	reason, err := recordActiveSessionTx(tx, worksetID)
	return reason != "", err
}

// recordActiveSessionTx names the queued or running session a record owns —
// workset.RefusalGenerating, workset.RefusalExecuting, or "" when it is idle.
func recordActiveSessionTx(tx *sql.Tx, worksetID string) (string, error) {
	for _, probe := range []struct {
		reason string
		query  string
	}{
		{
			workset.RefusalGenerating,
			`SELECT COUNT(*) FROM plan_generations WHERE workset_id = ? AND status IN ('queued','running')`,
		},
		{
			workset.RefusalExecuting,
			`SELECT COUNT(*) FROM plan_executions WHERE workset_id = ? AND status IN ('queued','running')`,
		},
	} {
		var n int
		if err := tx.QueryRow(probe.query, worksetID).Scan(&n); err != nil {
			return "", err
		}
		if n > 0 {
			return probe.reason, nil
		}
	}
	return "", nil
}

// deleteWorksetDataTx removes one record and everything it owns. Plans and
// Plans, executions and their payload rows cascade from the workset row.
func deleteWorksetDataTx(tx *sql.Tx, worksetID string) error {
	result, err := tx.Exec(`DELETE FROM worksets WHERE id = ?`, worksetID)
	if err != nil {
		return fmt.Errorf("delete record: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return workset.ErrWorksetNotFound
	}
	return nil
}

// InsertWorksetTx is the transaction-scoped form of the workset insert
// (members included); callers add their own operation rows.
func InsertWorksetTx(tx *sql.Tx, w *workset.Workset, members []workset.WorksetMember) error {
	return insertWorkset(tx, w, members)
}

func insertWorkset(tx *sql.Tx, w *workset.Workset, members []workset.WorksetMember) error {
	var libID, idemKey any
	if w.LibraryID != "" {
		libID = w.LibraryID
	}
	if w.CreationIdemKey != "" {
		idemKey = w.CreationIdemKey
	}
	if _, err := tx.Exec(`
		INSERT INTO worksets (id, title, library_id, root_path, root_path_key, version, creation_idem_key, creation_request_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, w.ID, w.Title, libID, w.RootPath, w.RootPathKey, w.Version, idemKey, w.CreationRequestHash, w.CreatedAt.Format(timeFormat), w.UpdatedAt.Format(timeFormat)); err != nil {
		if isUniqueConstraintError(err) {
			return workset.ErrWorksetIdemConflict
		}
		return fmt.Errorf("insert workset: %w", err)
	}
	for _, m := range members {
		if _, err := tx.Exec(`
			INSERT INTO workset_members (workset_id, member_id, member_index, rel_path, folder_path, folder_name)
			VALUES (?, ?, ?, ?, ?, ?)
		`, m.WorksetID, m.MemberID, m.MemberIndex, m.RelPath, m.FolderPath, m.FolderName); err != nil {
			return fmt.Errorf("insert workset member: %w", err)
		}
	}
	return nil
}

// ListWorksets lists worksets newest-first with keyset pagination on
// (updated_at, id). includeOrphaned=false excludes orphaned worksets. When
// libraryID is non-empty only worksets owned by that library are returned.
// The result set has at most limit rows; the caller derives the next cursor
// from the last row.
func (r *Repository) ListWorksets(
	cursorUpdatedAt string,
	cursorID string,
	limit int,
	libraryID string,
	includeOrphaned bool,
) ([]*workset.Workset, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT ` + worksetColumns + ` FROM worksets`
	var args []any
	var conds []string
	if libraryID != "" {
		conds = append(conds, "library_id = ?")
		args = append(args, libraryID)
	} else if !includeOrphaned {
		conds = append(conds, "library_id IS NOT NULL")
	}
	if cursorUpdatedAt != "" || cursorID != "" {
		conds = append(
			conds,
			`(julianday(updated_at) < julianday(?) OR (julianday(updated_at) = julianday(?) AND id < ?))`,
		)
		args = append(args, cursorUpdatedAt, cursorUpdatedAt, cursorID)
	}
	if len(conds) > 0 {
		query += " WHERE " + joinConds(conds)
	}
	query += ` ORDER BY julianday(updated_at) DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*workset.Workset
	for rows.Next() {
		w, err := scanWorkset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func joinConds(conds []string) string {
	out := conds[0]
	var outSb238 strings.Builder
	for _, c := range conds[1:] {
		outSb238.WriteString(" AND " + c)
	}
	out += outSb238.String()
	return out
}

// ListWorksetMembers returns the ordered members of a workset.
func (r *Repository) ListWorksetMembers(worksetID string) ([]*workset.WorksetMember, error) {
	rows, err := r.db.Query(`
		SELECT workset_id, member_id, member_index, rel_path, folder_path, folder_name
		FROM workset_members WHERE workset_id = ? ORDER BY member_index
	`, worksetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*workset.WorksetMember
	for rows.Next() {
		var m workset.WorksetMember
		if err := rows.Scan(
			&m.WorksetID,
			&m.MemberID,
			&m.MemberIndex,
			&m.RelPath,
			&m.FolderPath,
			&m.FolderName,
		); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// RenameWorkset updates the title with the metadata version guard. Returns
// workset.ErrWorksetNotFound when the row is absent and workset.ErrVersionConflict when the
// If-Match version is stale.
func (r *Repository) RenameWorkset(id, title string, expectedVersion int, now time.Time) error {
	result, err := r.db.Exec(`
		UPDATE worksets SET title = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND version = ?
	`, title, now.Format(timeFormat), id, expectedVersion)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return r.worksetVersionGuardResult(id)
	}
	return nil
}

// worksetVersionGuardResult distinguishes a stale If-Match from a missing
// workset.
func (r *Repository) worksetVersionGuardResult(id string) error {
	var n int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM worksets WHERE id = ?", id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return workset.ErrWorksetNotFound
	}
	return workset.ErrVersionConflict
}

// ==================== Library coordination ====================

// CountWorksetsForLibrary returns the number of worksets linked to a library.
func (r *Repository) CountWorksetsForLibrary(libraryID string) (int, error) {
	var n int
	err := r.db.QueryRow("SELECT COUNT(*) FROM worksets WHERE library_id = ?", libraryID).Scan(&n)
	return n, err
}

// LibraryRootPath returns the root path of a library.
func (r *Repository) LibraryRootPath(libraryID string) (string, error) {
	var p string
	err := r.db.QueryRow("SELECT root_path FROM libraries WHERE id = ?", libraryID).Scan(&p)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", library.ErrLibraryNotFound
		}
		return "", err
	}
	return p, nil
}
