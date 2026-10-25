package sqlite

import (
	"database/sql"
)

// mergeObservedEntries preserves directory references and unchanged measured facts.
// Type replacements happen before inserts so a new file always starts at revision 1.
func mergeObservedEntries(tx *sql.Tx, id, root string) error {
	if _, err := tx.Exec(`DELETE FROM inventory_files WHERE (dir_ref,name) IN (
  SELECT d.id,s.name FROM entries_staging s JOIN inventory_dirs d ON d.path=s.parent_path
  WHERE s.session_id=? AND s.is_dir=1)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM inventory_dirs WHERE path IN (
  SELECT path FROM entries_staging WHERE session_id=? AND is_dir=0)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO inventory_dirs(path,root_path,mtime,is_work_root)
  SELECT path,root_path,mtime,is_work_root FROM entries_staging WHERE session_id=? AND is_dir=1
  ON CONFLICT(path) DO UPDATE SET root_path=excluded.root_path,mtime=excluded.mtime,is_work_root=excluded.is_work_root
  WHERE inventory_dirs.root_path IS NOT excluded.root_path OR inventory_dirs.mtime IS NOT excluded.mtime
   OR inventory_dirs.is_work_root IS NOT excluded.is_work_root`, id); err != nil {
		return err
	}
	// Walks omit library and scope roots; file-only staging still needs its owning directory.
	if _, err := tx.Exec(`INSERT INTO inventory_dirs(path,root_path)
  SELECT root_path,root_path FROM entries_staging WHERE session_id=? AND root_path<>''
  UNION SELECT parent_path,root_path FROM entries_staging WHERE session_id=? AND is_dir=0 AND parent_path<>''
  ON CONFLICT(path) DO NOTHING`, id, id); err != nil {
		return err
	}
	if root != "" {
		if err := ensureInventoryDirectory(tx, root, root); err != nil {
			return err
		}
	}
	_, err := tx.Exec(mergeObservedFilesSQL, id)
	return err
}

func ensureInventoryDirectory(tx *sql.Tx, path, root string) error {
	parent, name := splitInventoryPath(path)
	if _, err := tx.Exec(
		`DELETE FROM inventory_files WHERE dir_ref=(SELECT id FROM inventory_dirs WHERE path=?) AND name=?`,
		parent,
		name,
	); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO inventory_dirs(path,root_path) VALUES(?,?) ON CONFLICT(path) DO NOTHING`, path, root)
	return err
}

// Unchanged observations do not rewrite files or their measured facts.
const mergeObservedFilesSQL = `
 INSERT INTO inventory_files(dir_ref,name,size,mtime,content_rev,bitrate,width,height,duration_sec,format)
 SELECT d.id,s.name,s.size,s.mtime,
  CASE WHEN f.name IS NULL THEN 1 WHEN f.size IS NOT s.size OR f.mtime IS NOT s.mtime THEN f.content_rev+1 ELSE f.content_rev END,
  CASE WHEN f.size IS NOT s.size OR f.mtime IS NOT s.mtime THEN NULL ELSE f.bitrate END,
  CASE WHEN f.size IS NOT s.size OR f.mtime IS NOT s.mtime THEN NULL ELSE f.width END,
  CASE WHEN f.size IS NOT s.size OR f.mtime IS NOT s.mtime THEN NULL ELSE f.height END,
  CASE WHEN f.size IS NOT s.size OR f.mtime IS NOT s.mtime THEN NULL ELSE f.duration_sec END,NULLIF(s.format,'')
 FROM entries_staging s JOIN inventory_dirs d ON d.path=s.parent_path
 LEFT JOIN inventory_files f ON f.dir_ref=d.id AND f.name=s.name
 WHERE s.session_id=? AND s.is_dir=0
 AND (f.name IS NULL OR f.size IS NOT s.size OR f.mtime IS NOT s.mtime OR f.format IS NOT NULLIF(s.format,''))
 ON CONFLICT(dir_ref,name) DO UPDATE SET size=excluded.size,mtime=excluded.mtime,content_rev=excluded.content_rev,
  bitrate=excluded.bitrate,width=excluded.width,height=excluded.height,duration_sec=excluded.duration_sec,format=excluded.format
`

// cleanupInventoryScope compares both path and type; directory removal cascades its files.
// A scope root is retained because folder walks report only its descendants.
func cleanupInventoryScope(tx *sql.Tx, id, root, scope string) error {
	dirRange := "root_path = ? AND path <> ?"
	fileRange := "d.root_path = ?"
	dirArgs := []any{root, root}
	fileArgs := []any{root}
	if scope != "" {
		from, to := subtree(scope)
		dirRange += " AND path >= ? AND path < ? AND path <> ?"
		dirArgs = append(dirArgs, from, to, scope)
		fileRange += " AND (d.path = ? OR (d.path >= ? AND d.path < ?))"
		fileArgs = append(fileArgs, scope, from, to)
	}
	dirArgs = append(dirArgs, id, id)
	if _, err := tx.Exec(`DELETE FROM inventory_dirs WHERE `+dirRange+` AND NOT EXISTS (
  SELECT 1 FROM entries_staging s WHERE s.session_id=? AND s.is_dir=1 AND s.path=inventory_dirs.path)
  AND NOT EXISTS (SELECT 1 FROM entries_staging s WHERE s.session_id=? AND s.is_dir=0 AND s.parent_path=inventory_dirs.path AND `+subtreeSQL("s.path", "inventory_dirs.path")+`)`, dirArgs...); err != nil {
		return err
	}
	fileArgs = append(fileArgs, id)
	_, err := tx.Exec(`DELETE FROM inventory_files WHERE (dir_ref,name) IN (
  SELECT d.id,f.name FROM inventory_dirs d CROSS JOIN inventory_files f
  WHERE f.dir_ref=d.id AND `+fileRange+` AND NOT EXISTS (
   SELECT 1 FROM entries_staging s WHERE s.session_id=? AND s.is_dir=0 AND s.path=`+joinInventoryPathSQL("d.path", "f.name")+`))`, fileArgs...)
	return err
}
