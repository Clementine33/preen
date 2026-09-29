package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// backupDatabase snapshots committed WAL content as well as the main file.
func backupDatabase(db *sql.DB, dbPath string) (string, error) {
	dir := filepath.Join(filepath.Dir(dbPath), "backups")
	if checkErr := os.MkdirAll(dir, 0700); checkErr != nil {
		return "", checkErr
	}
	file, err := os.CreateTemp(dir, filepath.Base(dbPath)+"-*.db")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if closeErr := file.Close(); closeErr != nil {
		os.Remove(path)
		return "", closeErr
	}
	if _, execErr := db.Exec("VACUUM INTO ?", path); execErr != nil {
		os.Remove(path)
		return "", execErr
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		os.Remove(path)
		return "", err
	}
	defer check.Close()
	var result string
	if queryErr := check.QueryRow("PRAGMA quick_check").Scan(&result); queryErr != nil {
		os.Remove(path)
		return "", queryErr
	}
	if result != "ok" {
		os.Remove(path)
		return "", fmt.Errorf("invalid database backup: %s", result)
	}
	return path, nil
}
