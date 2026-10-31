package sqlite

import (
	"database/sql"
)

// mergeObservedEntries preserves directory references and unchanged measured facts.
// Type replacements happen before inserts so a new file always starts at revision 1.
// scopes is a JSON array of selected folders limiting which staged rows are
// merged; empty merges the whole staging table.
func mergeObservedEntries(tx *sql.Tx, root, scopes string) error {
	in, inArgs := "1", []any(nil)
	if scopes != "" {
		in = `s.path IN (SELECT covered.path FROM json_each(?) scope CROSS JOIN temp.entries_staging covered
   WHERE ` + subtreeIncludingRootSQL("covered.path", "scope.value") + `)`
		inArgs = []any{scopes}
	}
	if _, err := tx.Exec(`DELETE FROM inventory_files WHERE (dir_ref,name) IN (
  SELECT d.id,s.name FROM temp.entries_staging s JOIN inventory_dirs d ON d.path=s.parent_path
  WHERE s.is_dir=1 AND `+in+`)`, inArgs...); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM inventory_dirs WHERE path IN (
  SELECT s.path FROM temp.entries_staging s WHERE s.is_dir=0 AND `+in+`)`, inArgs...); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO inventory_dirs(path,root_path,mtime,is_work_root)
  SELECT s.path,?,s.mtime,s.is_work_root FROM temp.entries_staging s WHERE s.is_dir=1 AND `+in+`
  ON CONFLICT(path) DO UPDATE SET root_path=excluded.root_path,mtime=excluded.mtime,is_work_root=excluded.is_work_root
  WHERE inventory_dirs.root_path IS NOT excluded.root_path OR inventory_dirs.mtime IS NOT excluded.mtime
   OR inventory_dirs.is_work_root IS NOT excluded.is_work_root`, append([]any{root}, inArgs...)...); err != nil {
		return err
	}
	// Scoped walks omit their root; file-only staging still needs its owning directory.
	if _, err := tx.Exec(`INSERT INTO inventory_dirs(path,root_path)
 SELECT s.parent_path,? FROM temp.entries_staging s WHERE s.is_dir=0 AND s.parent_path<>'' AND `+in+`
 ON CONFLICT(path) DO NOTHING`, append([]any{root}, inArgs...)...); err != nil {
		return err
	}
	if root != "" {
		if err := ensureInventoryDirectory(tx, root, root); err != nil {
			return err
		}
	}
	_, err := tx.Exec(mergeObservedFilesSQL(in), inArgs...)
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
func mergeObservedFilesSQL(in string) string {
	return `
 INSERT INTO inventory_files(dir_ref,name,size,mtime,format)
 SELECT d.id,s.name,s.size,s.mtime,NULLIF(s.format,'')
 FROM temp.entries_staging s JOIN inventory_dirs d ON d.path=s.parent_path
 WHERE s.is_dir=0 AND ` + in + observedFileUpsertSQL
}

// Scans and exact synchronization share the same revision and measured-fact rules.
const observedFileUpsertSQL = `
 ON CONFLICT(dir_ref,name) DO UPDATE SET
  size=excluded.size,mtime=excluded.mtime,format=excluded.format,
  content_rev=inventory_files.content_rev+CASE WHEN ` + observedContentChangedSQL + ` THEN 1 ELSE 0 END,
  bitrate=CASE WHEN ` + observedContentChangedSQL + ` THEN NULL ELSE inventory_files.bitrate END,
  width=CASE WHEN ` + observedContentChangedSQL + ` THEN NULL ELSE inventory_files.width END,
  height=CASE WHEN ` + observedContentChangedSQL + ` THEN NULL ELSE inventory_files.height END,
  duration_sec=CASE WHEN ` + observedContentChangedSQL + ` THEN NULL ELSE inventory_files.duration_sec END
 WHERE ` + observedContentChangedSQL + ` OR inventory_files.format IS NOT excluded.format
`

const observedContentChangedSQL = `inventory_files.size IS NOT excluded.size OR inventory_files.mtime IS NOT excluded.mtime`

// cleanupInventoryScope compares both path and type; directory removal cascades its files.
// A scope root is retained because folder walks report only its descendants.
func cleanupInventoryScope(tx *sql.Tx, root, scope string) error {
	dirRange := "root_path = ? AND path <> ?"
	fileRange := "d.root_path = ?"
	dirArgs := []any{root, root}
	fileArgs := []any{root}
	if scope != "" {
		// Folder walks omit the scope root's attributes, so its refreshed listing has no paired mtime.
		if _, err := tx.Exec(`UPDATE inventory_dirs SET mtime=0 WHERE path=? AND mtime<>0
 AND NOT EXISTS (SELECT 1 FROM temp.entries_staging WHERE is_dir=1 AND path=?)`, scope, scope); err != nil {
			return err
		}
		from, to := subtree(scope)
		dirRange += " AND path >= ? AND path < ? AND path <> ?"
		dirArgs = append(dirArgs, from, to, scope)
		// Outer bounds keep the root_path/path index range; without them the OR
		// leaves only root_path=? and every scope scans the whole library.
		fileRange += " AND d.path >= ? AND d.path < ? AND (d.path = ? OR (d.path >= ? AND d.path < ?))"
		fileArgs = append(fileArgs, scope, to, scope, from, to)
	}
	if _, err := tx.Exec(`DELETE FROM inventory_dirs WHERE `+dirRange+` AND NOT EXISTS (
  SELECT 1 FROM temp.entries_staging s WHERE s.is_dir=1 AND s.path=inventory_dirs.path)
  AND NOT EXISTS (SELECT 1 FROM temp.entries_staging s WHERE s.is_dir=0 AND s.parent_path=inventory_dirs.path AND `+subtreeSQL("s.path", "inventory_dirs.path")+`)`, dirArgs...); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM inventory_files WHERE (dir_ref,name) IN (
  SELECT d.id,f.name FROM inventory_dirs d CROSS JOIN inventory_files f
  WHERE f.dir_ref=d.id AND `+fileRange+` AND NOT EXISTS (
   SELECT 1 FROM temp.entries_staging s WHERE s.is_dir=0 AND s.path=`+joinInventoryPathSQL("d.path", "f.name")+`)
  AND NOT EXISTS (SELECT 1 FROM temp.entries_staging s WHERE s.is_dir=1 AND s.carried=1 AND s.path=d.path))`, fileArgs...)
	return err
}
