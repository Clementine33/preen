package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// RebuildDevelopmentDatabase backs up and replaces a stopped development
// database. Legacy development files can be rebuilt; release files are refused.
func RebuildDevelopmentDatabase(dbPath string) (string, error) {
	if checkErr := EnsureDBPath(dbPath); checkErr != nil {
		return "", checkErr
	}
	backup, err := backupDevelopmentDatabase(dbPath)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp(filepath.Dir(dbPath), "rebuild-*.db")
	if err != nil {
		return backup, err
	}
	path := file.Name()
	defer os.Remove(path)
	if closeErr := file.Close(); closeErr != nil {
		return backup, closeErr
	}
	repo, err := NewDevelopmentRepository(path)
	if err != nil {
		return backup, err
	}
	if closeErr := repo.Close(); closeErr != nil {
		return backup, closeErr
	}
	// The backend must be stopped: old sidecars must never accompany the new file.
	for _, suffix := range []string{"-wal", "-shm"} {
		if removeErr := os.Remove(dbPath + suffix); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return backup, removeErr
		}
	}
	return backup, os.Rename(path, dbPath)
}

func backupDevelopmentDatabase(dbPath string) (string, error) {
	if _, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	tables, err := userTables(db)
	if err != nil {
		return "", err
	}
	if _, ok := tables["goose_db_version"]; ok {
		return "", fmt.Errorf("%w: cannot rebuild a release database", ErrIncompatibleDatabase)
	}
	return backupDatabase(db, dbPath)
}
