package sqlite

import (
	"database/sql"
	"errors"
	"path/filepath"
	"time"

	"github.com/Clementine33/preen/backend/internal/inventory"
)

// LoadDirectoryReuse limits reuse to observations older than a retained successful
// full-root scan. A missing retained session requires a fresh observation.
func (a *ScanStaging) LoadDirectoryReuse(
	root string,
	margin time.Duration,
) (map[string]inventory.DirectoryReuse, error) {
	var started string
	err := a.repo.db.QueryRow(`SELECT started_at FROM scan_sessions
 WHERE root_path=? AND kind='full' AND status='completed' AND scope_path IS NULL
 ORDER BY started_at DESC LIMIT 1`, root).Scan(&started)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cutoff := parseTimestamp(started).Add(-margin)
	cutoffSeconds := float64(cutoff.Unix()) + float64(cutoff.Nanosecond())/1e9
	rows, err := a.repo.db.Query(`SELECT d.path,d.mtime,
 (SELECT COUNT(*) FROM inventory_files f WHERE f.dir_ref=d.id),d.mtime>0 AND d.mtime<?
 FROM inventory_dirs d WHERE d.root_path=?`, cutoffSeconds, root)
	if err != nil {
		return nil, err
	}
	out := make(map[string]inventory.DirectoryReuse)
	var directories []string
	for rows.Next() {
		var path string
		var d inventory.DirectoryReuse
		var eligible bool
		if err = rows.Scan(&path, &d.Mtime, &d.FileCount, &eligible); err != nil {
			rows.Close()
			return nil, err
		}
		directories = append(directories, path)
		if eligible {
			out[path] = d
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, path := range directories {
		if path == root {
			continue
		}
		parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(path)))
		if d, ok := out[parent]; ok {
			if d.Entries == nil {
				d.Entries = make(map[string]bool)
			}
			d.Entries[filepath.Base(filepath.FromSlash(path))] = true
			out[parent] = d
		}
	}
	rows, err = a.repo.db.Query(`SELECT d.path,f.name
 FROM inventory_dirs d JOIN inventory_files f ON f.dir_ref=d.id
 WHERE d.root_path=? AND d.mtime>0 AND d.mtime<?
 AND EXISTS (SELECT 1 FROM inventory_dirs child WHERE child.root_path=d.root_path
 AND `+subtreeSQL("child.path", "d.path")+`)`, root, cutoffSeconds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var path, name string
		if err := rows.Scan(&path, &name); err != nil {
			return nil, err
		}
		if d := out[path]; d.Entries != nil {
			d.Entries[name] = false
		}
	}
	return out, rows.Err()
}
