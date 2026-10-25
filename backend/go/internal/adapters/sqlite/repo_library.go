package sqlite

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/library"
	"github.com/Clementine33/preen/backend/internal/librarypath"
)

// ==================== Types ====================

// isUniqueConstraintError reports whether err is a SQLite UNIQUE constraint
// violation.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "constraint") && strings.Contains(errStr, "unique")
}

// ==================== Library CRUD ====================

// CreateLibrary inserts a new library. rootPath is stored as the cleaned
// canonical root (POSIX separators, lexical cleaning); uniqueness is enforced
// on the canonical identity key, so equivalent spellings (`/music/.`,
// `C:/Music` vs `c:/music`) conflict.
func (r *Repository) CreateLibrary(name, rootPath string) (*library.Library, error) {
	rootPath = librarypath.CleanRootPath(rootPath)
	rootPathKey := librarypath.RootPathKey(rootPath)
	now := time.Now().Format(timeFormat)
	id := uuid.NewString()
	_, err := r.db.Exec(`
		INSERT INTO libraries (id, name, root_path, root_path_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, name, rootPath, rootPathKey, now, now)
	if err != nil {
		if isUniqueConstraintError(err) {
			return nil, library.ErrLibraryExists
		}
		return nil, err
	}
	return r.GetLibrary(id)
}

// ListLibraries returns all libraries ordered by created_at (newest first).
func (r *Repository) ListLibraries() ([]*library.Library, error) {
	rows, err := r.db.Query(`
		SELECT id, name, root_path, created_at, updated_at, last_scan_at, last_scan_status, last_scan_error, storage_class
		FROM libraries ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var libraries []*library.Library
	for rows.Next() {
		lib, err := scanLibrary(rows)
		if err != nil {
			return nil, err
		}
		libraries = append(libraries, lib)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return libraries, nil
}

// GetLibrary retrieves a library by id.
func (r *Repository) GetLibrary(id string) (*library.Library, error) {
	row := r.db.QueryRow(`
		SELECT id, name, root_path, created_at, updated_at, last_scan_at, last_scan_status, last_scan_error, storage_class
		FROM libraries WHERE id = ?
	`, id)
	lib, err := scanLibrary(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, library.ErrLibraryNotFound
		}
		return nil, err
	}
	return lib, nil
}

// ErrLibraryHasWorksets is returned when a root-path change is attempted on a
// library that still owns worksets. workset.Workset membership identity is a
// normalized library-relative path, so silently rebinding the root would
// reattach fixed worksets to unrelated content.

// ErrGenerationInProgress is returned when an owned workset has a queued or
// running planning session and an operation that must wait for it (library
// deletion) is attempted.

// UpdateLibrary updates a library's name and root path and returns the
// updated row. Changing the root drops the scanned inventory of the old root
// and the prior scan state in the same transaction, so no stale paths remain
// attached and the next scan starts from nothing. A root-path change is
// rejected with ErrLibraryHasWorksets while a processing record still belongs
// to the library (a record keeps its root until it is replaced); name
// edits stay allowed. Omitted fields retain their value inside this transaction.
func (r *Repository) UpdateLibrary(
	id string,
	name, rootPath *string,
	storageClass *library.StorageClass,
) (*library.Library, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var currentRoot string
	if err := tx.QueryRow("SELECT root_path FROM libraries WHERE id = ?", id).Scan(&currentRoot); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, library.ErrLibraryNotFound
		}
		return nil, err
	}

	candidateRoot := currentRoot
	if rootPath != nil {
		candidateRoot = librarypath.CleanRootPath(*rootPath)
	}
	rootPathKey := librarypath.RootPathKey(candidateRoot)
	now := time.Now().Format(timeFormat)
	// A root change is judged on the canonical identity key, so a spelling-only
	// edit (e.g. `/music/.` for `/music`) must not invalidate folders or scan
	// state, while `C:/Music` -> `c:/music` is genuinely unchanged on Windows.
	rootChanged := librarypath.RootPathKey(currentRoot) != rootPathKey
	if rootChanged {
		var n int
		if err := tx.QueryRow("SELECT COUNT(*) FROM worksets WHERE library_id = ?", id).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			return nil, library.ErrLibraryHasWorksets
		}
	}
	query := `UPDATE libraries SET name = COALESCE(?, name), root_path = ?, root_path_key = ?, updated_at = ?, storage_class = COALESCE(?, storage_class) WHERE id = ?`
	if rootChanged {
		query = `
			UPDATE libraries
			SET name = COALESCE(?, name), root_path = ?, root_path_key = ?, updated_at = ?,
			    last_scan_at = NULL, last_scan_status = '', last_scan_error = '', storage_class = COALESCE(?, storage_class)
			WHERE id = ?
		`
	}
	if _, err := tx.Exec(query, name, candidateRoot, rootPathKey, now, storageClass, id); err != nil {
		if isUniqueConstraintError(err) {
			return nil, library.ErrLibraryExists
		}
		return nil, err
	}
	if rootChanged {
		// The inventory of the previous root is not the inventory of the new
		// one: drop it and let the next scan rebuild it, so nothing reads a
		// stale listing as if it described the new root. The bindings go with
		// it - they name directories under the old root, so keeping them would
		// point the catalog at paths that are no longer part of this library.
		if _, err := tx.Exec("DELETE FROM entries WHERE root_path = ?", currentRoot); err != nil {
			return nil, err
		}
		if err := unbindLibraryCopies(tx, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if rootChanged {
		r.catalogChanged()
	}
	return r.GetLibrary(id)
}

// DeleteLibrary removes a library together with every processing record it
// owns, in one transaction: the records' plans, executions, members, drafts and
// sessions go with them, so no orphaned record survives its library. Media
// files and recovery directories on disk are never touched.
//
// It fails with ErrGenerationInProgress while a record has a queued or running
// planning session and with ErrExecutionInProgress while one has a queued or
// running execution (the client cancels first), and with ErrLibraryNotFound
// when the library does not exist. The active-session checks happen inside the
// write transaction: claim/complete updates serialize on the same SQLite
// writer, so a delete that commits cannot race a session that would start
// against the deleted library.
func (r *Repository) DeleteLibrary(id string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists int
	if countErr := tx.QueryRow("SELECT COUNT(*) FROM libraries WHERE id = ?", id).Scan(&exists); countErr != nil {
		return countErr
	}
	if exists == 0 {
		return library.ErrLibraryNotFound
	}

	var active int
	if countErr := tx.QueryRow(`
		SELECT COUNT(*) FROM plan_generations g
		JOIN worksets w ON w.id = g.workset_id
		WHERE w.library_id = ? AND g.status IN ('queued','running')
	`, id).Scan(&active); countErr != nil {
		return countErr
	}
	if active > 0 {
		return library.ErrGenerationInProgress
	}

	if countErr := tx.QueryRow(`
		SELECT COUNT(*) FROM plan_executions e
		JOIN worksets w ON w.id = e.workset_id
		WHERE w.library_id = ? AND e.status IN ('queued','running')
	`, id).Scan(&active); countErr != nil {
		return countErr
	}
	if active > 0 {
		return library.ErrExecutionInProgress
	}

	rows, err := tx.Query("SELECT id FROM worksets WHERE library_id = ?", id)
	if err != nil {
		return err
	}
	var recordIDs []string
	for rows.Next() {
		var recordID string
		if scanErr := rows.Scan(&recordID); scanErr != nil {
			rows.Close()
			return scanErr
		}
		recordIDs = append(recordIDs, recordID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, recordID := range recordIDs {
		if _, err := tx.Exec("DELETE FROM worksets WHERE id = ?", recordID); err != nil {
			return err
		}
	}
	// Deleting the library row would cascade its bindings away as well, but the
	// removal is explicit here so the rule reads the same as the root change:
	// the bindings describe directories under this library and go with it. The
	// works and their personal data stay.
	if err := unbindLibraryCopies(tx, id); err != nil {
		return err
	}
	// As with a root change, the inventory of the root goes with the library;
	// no other library's scan would ever remove it.
	if _, err := tx.Exec(
		"DELETE FROM entries WHERE root_path = (SELECT root_path FROM libraries WHERE id = ?)",
		id,
	); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM libraries WHERE id = ?", id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.catalogChanged()
	return nil
}

// UpdateLibraryScanState records the outcome of a library scan.
func (r *Repository) UpdateLibraryScanState(id, status, errMsg string, finishedAt time.Time) error {
	_, err := r.db.Exec(`
		UPDATE libraries SET last_scan_status = ?, last_scan_error = ?, last_scan_at = ?
		WHERE id = ?
	`, status, errMsg, finishedAt.Format(timeFormat), id)
	return err
}

// scanLibrary decodes a library row from a row or rows-based scanner.
func scanLibrary(row interface{ Scan(...any) error }) (*library.Library, error) {
	var l library.Library
	var createdAtStr, updatedAtStr string
	var lastScanAt sql.NullString
	if err := row.Scan(
		&l.ID,
		&l.Name,
		&l.RootPath,
		&createdAtStr,
		&updatedAtStr,
		&lastScanAt,
		&l.LastScanStatus,
		&l.LastScanError,
		&l.StorageClass,
	); err != nil {
		return nil, err
	}
	l.CreatedAt = parseTimestamp(createdAtStr)
	l.UpdatedAt = parseTimestamp(updatedAtStr)
	if lastScanAt.Valid && lastScanAt.String != "" {
		t := parseTimestamp(lastScanAt.String)
		l.LastScanAt = &t
	}
	return &l, nil
}

// ==================== Overview listing ====================

// ListLibraryDirs returns the scanned work roots of a library root that
// the workbench overview shows: every directory the last scan saw, whether or
// not it holds audio, with the audio and file counts of its subtree. The
// recovery directories at every depth are not members and are left out. Order is
// by path, so the caller sees a stable listing.
//
// The listing comes from the scanned inventory rather than from the disk, so
// it describes exactly the state the rest of the workbench reads, and its
// identity is the library-relative path, which a rescan cannot change.
func (r *Repository) ListLibraryDirs(rootPath string) ([]*inventory.LibraryDir, error) {
	rootPath = librarypath.CleanRootPath(rootPath)
	rows, err := r.db.Query(`
		SELECT d.path, d.name, d.created_at,
		       (SELECT COUNT(*) FROM entries f
		         WHERE f.root_path = d.root_path AND f.is_dir = 0 AND `+subtreeSQL("f.path", "d.path")+` AND f.is_audio = 1) AS audio_count,
		       (SELECT COUNT(*) FROM entries f
		         WHERE f.root_path = d.root_path AND f.is_dir = 0 AND `+subtreeSQL("f.path", "d.path")+`) AS file_count
		FROM entries d
		WHERE d.is_dir = 1
		  AND d.root_path = ? AND d.is_work_root = 1
		ORDER BY d.path
	`, rootPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prefix := joinInventoryPath(rootPath, "")
	var out []*inventory.LibraryDir
	for rows.Next() {
		var d inventory.LibraryDir
		var addedAt sql.NullInt64
		if err := rows.Scan(&d.Path, &d.Name, &addedAt, &d.AudioFileCount, &d.FileCount); err != nil {
			return nil, err
		}
		if addedAt.Valid {
			d.AddedAt = time.Unix(addedAt.Int64, 0).UTC()
		}
		d.RelPath = strings.TrimPrefix(librarypath.NormalizeToPOSIX(d.Path), prefix)
		if librarypath.IsRecoveryRelPath(rootPath, d.RelPath) {
			continue
		}
		out = append(out, &d)
	}
	return out, rows.Err()
}

// ListLibraryWorkRoots returns the library-relative path of every scanned work
// root. It carries no subtree
// statistics: resolving a directory identity needs the identities only, not the
// counts the overview listing shows. Recovery directories at every depth are not
// members and are left out, exactly as in the overview listing.
func (r *Repository) ListLibraryWorkRoots(rootPath string) ([]string, error) {
	rootPath = librarypath.CleanRootPath(rootPath)
	rows, err := r.db.Query(`
		SELECT path, name
		FROM entries
		WHERE is_dir = 1
		  AND root_path = ? AND is_work_root = 1
		ORDER BY path
	`, rootPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prefix := joinInventoryPath(rootPath, "")
	var out []string
	for rows.Next() {
		var path, name string
		if err := rows.Scan(&path, &name); err != nil {
			return nil, err
		}
		if librarypath.IsRecoveryRelPath(
			rootPath,
			strings.TrimPrefix(librarypath.NormalizeToPOSIX(path), prefix),
		) {
			continue
		}
		out = append(out, strings.TrimPrefix(librarypath.NormalizeToPOSIX(path), prefix))
	}
	return out, rows.Err()
}

// DirAudioCounts returns the subtree audio-file count of each requested
// library-relative directory, keyed by the relative path as given. A path the
// scanned inventory does not know as a directory of the library root is absent
// from the result — the caller resolves that absence as "not a member
// directory" rather than as the count zero.
func (r *Repository) DirAudioCounts(rootPath string, relPaths []string) (map[string]int, error) {
	out := make(map[string]int, len(relPaths))
	if len(relPaths) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(relPaths)+1)
	placeholders := make([]string, 0, len(relPaths))
	byPath := make(map[string]string, len(relPaths))
	for _, rel := range relPaths {
		if _, ok := byPath[rel]; ok {
			continue
		}
		abs := librarypath.JoinRel(rootPath, rel)
		byPath[abs] = rel
		args = append(args, abs)
		placeholders = append(placeholders, "?")
	}
	args = append(args, librarypath.CleanRootPath(rootPath))
	rows, err := r.db.Query(`
		SELECT d.path,
		       (SELECT COUNT(*) FROM entries f
		         WHERE f.root_path = d.root_path AND f.is_dir = 0 AND `+subtreeSQL("f.path", "d.path")+` AND f.is_audio = 1) AS audio_count
		FROM entries d
		WHERE d.is_dir = 1 AND d.path IN (`+strings.Join(placeholders, ", ")+`) AND d.root_path = ?
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var abs string
		var count int
		if err := rows.Scan(&abs, &count); err != nil {
			return nil, err
		}
		if rel, ok := byPath[abs]; ok {
			out[rel] = count
		}
	}
	return out, rows.Err()
}

// ListEntriesUnderPath returns entries under a path prefix (including the
// prefix itself) for tree building. Path identity is binary and slash-boundary
// exact, so folder names like "100%_hits" match only their own subtree and
// case-distinct siblings stay distinct.
func (r *Repository) ListEntriesUnderPath(pathPrefix string) ([]inventory.Entry, error) {
	pathPrefix = librarypath.CleanRootPath(pathPrefix)
	from, to := subtree(pathPrefix)
	rows, err := r.db.Query(`
		SELECT path, parent_path, name, is_dir, size, mtime, bitrate, format, width, height, duration_sec, content_rev
		FROM entries
		WHERE path = ? OR (path >= ? AND path < ?)
		ORDER BY path
	`, pathPrefix, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []inventory.Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// EntryByPath reads one file's stored row (ADR 0013): the whitelist the
// content endpoint serves from — a path with no inventory row is not served,
// whatever the disk holds.
func (r *Repository) EntryByPath(path string) (inventory.Entry, bool, error) {
	rows, err := r.db.Query(`
		SELECT path, parent_path, name, is_dir, size, mtime, bitrate, format, width, height, duration_sec, content_rev
		FROM entries
		WHERE path = ? AND is_dir = 0
	`, librarypath.NormalizeToPOSIX(path))
	if err != nil {
		return inventory.Entry{}, false, err
	}
	defer rows.Close()

	if !rows.Next() {
		return inventory.Entry{}, false, rows.Err()
	}
	entry, scanErr := scanEntry(rows)
	if scanErr != nil {
		return inventory.Entry{}, false, scanErr
	}
	return entry, true, rows.Err()
}

// scanEntry reads one entries row in the tree projection's column order.
func scanEntry(rows *sql.Rows) (inventory.Entry, error) {
	var e inventory.Entry
	var isDir int
	var bitrate sql.NullInt32
	var format sql.NullString
	var width, height sql.NullInt64
	var duration sql.NullFloat64
	if err := rows.Scan(
		&e.Path, &e.ParentPath, &e.Name, &isDir, &e.Size, &e.Mtime,
		&bitrate, &format, &width, &height, &duration, &e.ContentRev,
	); err != nil {
		return inventory.Entry{}, err
	}
	e.IsDir = isDir == 1
	if bitrate.Valid {
		v := bitrate.Int32
		e.Bitrate = &v
	}
	e.Format = format.String
	if width.Valid && height.Valid {
		w, h := int(width.Int64), int(height.Int64)
		e.Width, e.Height = &w, &h
	}
	if duration.Valid {
		sec := duration.Float64
		e.DurationSec = &sec
	}
	return e, nil
}
