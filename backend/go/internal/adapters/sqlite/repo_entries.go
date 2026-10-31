package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/Clementine33/preen/backend/internal/librarypath"

	"github.com/Clementine33/preen/backend/internal/inventory"
)

// ObservedAudioEntries reads the observed audio files at or beneath one root,
// each with the generation credential recorded for it when there is one. The
// subtree uses the stored POSIX path's binary range, preserving case and treating
// '%' and '_' literally while searching the path index.
func (r *Repository) ObservedAudioEntries(rootPath string) ([]inventory.ObservedAudio, error) {
	rootPath = librarypath.CleanRootPath(rootPath)
	from, to := subtree(rootPath)
	parent, name := splitInventoryPath(rootPath)
	filePath := joinInventoryPathSQL("f.dir_path", "f.name")
	rows, err := r.db.Query(`
		WITH scoped_files AS (
			SELECT d.path AS dir_path, f.name, f.size, f.mtime, f.bitrate, f.format
			FROM inventory_dirs d JOIN inventory_files f ON f.dir_ref = d.id
			WHERE d.path = ? OR (d.path >= ? AND d.path < ?)
			UNION ALL
			SELECT d.path, f.name, f.size, f.mtime, f.bitrate, f.format
			FROM inventory_dirs d JOIN inventory_files f ON f.dir_ref = d.id
			WHERE d.path = ? AND f.name = ?
		)
		SELECT `+filePath+`, f.size, f.mtime, COALESCE(f.bitrate, 0), COALESCE(f.format, ''),
		       g.codec, g.bitrate_kbps, g.mode, g.size, g.mtime
		FROM scoped_files f LEFT JOIN generation_records g ON g.path = `+filePath+`
		ORDER BY 1
	`, rootPath, from, to, parent, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]inventory.ObservedAudio, 0)
	for rows.Next() {
		e, err := scanObservedAudio(rows)
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

var observedAudioPathSQL = joinInventoryPathSQL("f.dir_path", "f.name")

// observedAudioByRootSQL is ObservedAudioEntries for many roots at once, but
// only the is_audio files - a superset of the planner's audio extensions, so
// the caller's exact filter still decides while sidecars stay in the database.
// The roots arrive as one JSON array and each leg stays a path-index range
// search per root, so no root reads the rest of the library: CROSS JOIN pins
// that loop order, which the is_audio term would otherwise tempt the planner
// into inverting as a scan of every file.
var observedAudioByRootSQL = `
	WITH roots AS (
		SELECT json_extract(value, '$.key') AS key, json_extract(value, '$.root') AS root,
		       json_extract(value, '$.from') AS lo, json_extract(value, '$.to') AS hi,
		       json_extract(value, '$.parent') AS parent, json_extract(value, '$.name') AS name
		FROM json_each(?)
	), scoped_files AS (
		SELECT r.key, d.path AS dir_path, f.name, f.size, f.mtime, f.bitrate, f.format
		FROM roots r CROSS JOIN inventory_dirs d ON d.path = r.root
		CROSS JOIN inventory_files f ON f.dir_ref = d.id AND f.is_audio = 1
		UNION ALL
		SELECT r.key, d.path, f.name, f.size, f.mtime, f.bitrate, f.format
		FROM roots r CROSS JOIN inventory_dirs d ON d.path >= r.lo AND d.path < r.hi
		CROSS JOIN inventory_files f ON f.dir_ref = d.id AND f.is_audio = 1
		UNION ALL
		SELECT r.key, d.path, f.name, f.size, f.mtime, f.bitrate, f.format
		FROM roots r CROSS JOIN inventory_dirs d ON d.path = r.parent
		CROSS JOIN inventory_files f ON f.dir_ref = d.id AND f.name = r.name AND f.is_audio = 1
	)
	SELECT f.key, ` + observedAudioPathSQL + `, f.size, f.mtime, COALESCE(f.bitrate, 0), COALESCE(f.format, ''),
	       g.codec, g.bitrate_kbps, g.mode, g.size, g.mtime
	FROM scoped_files f LEFT JOIN generation_records g ON g.path = ` + observedAudioPathSQL + `
	ORDER BY 2
`

// ObservedAudioEntriesByRoot reads the observed audio candidates under every
// given root in one query, keyed by the root as passed, each list ordered by
// path. A root with no entries has no key.
func (r *Repository) ObservedAudioEntriesByRoot(roots []string) (map[string][]inventory.ObservedAudio, error) {
	args := make([]map[string]string, 0, len(roots))
	for _, key := range roots {
		root := librarypath.CleanRootPath(key)
		from, to := subtree(root)
		parent, name := splitInventoryPath(root)
		args = append(args, map[string]string{
			"key": key, "root": root, "from": from, "to": to, "parent": parent, "name": name,
		})
	}
	payload, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(observedAudioByRootSQL, string(payload))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]inventory.ObservedAudio, len(roots))
	for rows.Next() {
		var key string
		e, err := scanObservedAudio(rows, &key)
		if err != nil {
			return nil, err
		}
		out[key] = append(out[key], e)
	}
	return out, rows.Err()
}

// scanObservedAudio scans one observed-audio row, after any leading columns
// the caller selected in front of it.
func scanObservedAudio(rows *sql.Rows, lead ...any) (inventory.ObservedAudio, error) {
	var e inventory.ObservedAudio
	var codec, mode sql.NullString
	var bitrateKbps, recordSize, recordMtime sql.NullInt64
	dest := slices.Concat(lead, []any{&e.Path, &e.Size, &e.Mtime, &e.Bitrate, &e.Format,
		&codec, &bitrateKbps, &mode, &recordSize, &recordMtime})
	if err := rows.Scan(dest...); err != nil {
		return e, err
	}
	if codec.Valid {
		e.Generated = &inventory.GeneratedCredential{
			Codec:       codec.String,
			BitrateKbps: int(bitrateKbps.Int64),
			Mode:        mode.String,
			Size:        recordSize.Int64,
			Mtime:       recordMtime.Int64,
		}
	}
	return e, nil
}

// RootExistsInInventory reports whether the planning root itself is present in
// the scanned entries (directory or file row). A folder that was never scanned,
// or whose scan removed it, is absent.
func (r *Repository) RootExistsInInventory(rootPath string) (bool, error) {
	rootPath = librarypath.NormalizeToPOSIX(rootPath)
	dir, name := splitInventoryPath(rootPath)
	var n int
	err := r.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM inventory_dirs WHERE path = ?)
		    OR EXISTS(SELECT 1 FROM inventory_dirs d JOIN inventory_files f ON f.dir_ref = d.id
		              WHERE d.path = ? AND f.name = ?)
	`, rootPath, dir, name).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check root existence: %w", err)
	}
	return n > 0, nil
}

// ListEntriesUnderPath returns entries under a path prefix (including the
// prefix itself) for tree building. Path identity is binary and slash-boundary
// exact, so folder names like "100%_hits" match only their own subtree and
// case-distinct siblings stay distinct.
func (r *Repository) ListEntriesUnderPath(pathPrefix string) ([]inventory.Entry, error) {
	pathPrefix = librarypath.CleanRootPath(pathPrefix)
	from, to := subtree(pathPrefix)
	parent, name := splitInventoryPath(pathPrefix)
	rows, err := r.db.Query(`
		SELECT d.path, '', 1, 0, d.mtime, NULL, NULL, NULL, NULL, NULL, 0
		FROM inventory_dirs d
		WHERE d.path != d.root_path AND (d.path = ? OR (d.path >= ? AND d.path < ?))
		UNION ALL
		SELECT d.path, f.name, 0, f.size, f.mtime, f.bitrate, f.format, f.width, f.height, f.duration_sec, f.content_rev
		FROM inventory_dirs d JOIN inventory_files f ON f.dir_ref = d.id
		WHERE d.path = ? OR (d.path >= ? AND d.path < ?)
		UNION ALL
		SELECT d.path, f.name, 0, f.size, f.mtime, f.bitrate, f.format, f.width, f.height, f.duration_sec, f.content_rev
		FROM inventory_dirs d JOIN inventory_files f ON f.dir_ref = d.id
		WHERE d.path = ? AND f.name = ?
	`, pathPrefix, from, to, pathPrefix, from, to, parent, name)
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
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// EntryByPath reads one file's stored row (ADR 0013): the whitelist the
// content endpoint serves from — a path with no inventory row is not served,
// whatever the disk holds.
func (r *Repository) EntryByPath(path string) (inventory.Entry, bool, error) {
	dir, name := splitInventoryPath(librarypath.NormalizeToPOSIX(path))
	rows, err := r.db.Query(`
		SELECT d.path, f.name, 0, f.size, f.mtime, f.bitrate, f.format, f.width, f.height, f.duration_sec, f.content_rev
		FROM inventory_dirs d JOIN inventory_files f ON f.dir_ref = d.id
		WHERE d.path = ? AND f.name = ?
	`, dir, name)
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

// scanEntry maps stored directory/file projections back to path-based inventory facts.
func scanEntry(rows *sql.Rows) (inventory.Entry, error) {
	var e inventory.Entry
	var isDir int
	var bitrate sql.NullInt32
	var format sql.NullString
	var width, height sql.NullInt64
	var duration sql.NullFloat64
	if err := rows.Scan(
		&e.ParentPath, &e.Name, &isDir, &e.Size, &e.Mtime,
		&bitrate, &format, &width, &height, &duration, &e.ContentRev,
	); err != nil {
		return inventory.Entry{}, err
	}
	e.IsDir = isDir == 1
	if e.IsDir {
		e.Path = e.ParentPath
		e.ParentPath, e.Name = splitInventoryPath(e.Path)
	} else {
		e.Path = joinInventoryPath(e.ParentPath, e.Name)
	}
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
