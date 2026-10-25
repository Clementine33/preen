package sqlite

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

// SetTempDirectory configures SQLite's process-global spill directory. Call once
// at process startup, before opening any connection; SQLite does not synchronize it.
func SetTempDirectory(directory string) error {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	if strings.ContainsRune(directory, 0) {
		return fmt.Errorf("SQLite temp directory contains NUL")
	}
	info, err := os.Stat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("SQLite temp path is not a directory: %s", directory)
	}
	tls := libc.NewTLS()
	defer tls.Close()
	source, err := libc.CString(directory)
	if err != nil {
		return err
	}
	defer libc.Xfree(tls, source)
	target := sqlite3.Xsqlite3_malloc64(tls, uint64(len(directory)+1))
	if target == 0 {
		return fmt.Errorf("allocate SQLite temp directory")
	}
	libc.Xstrcpy(tls, target, source)
	sqlite3.Xsqlite3_free(tls, sqlite3.Xsqlite3_temp_directory)
	sqlite3.Xsqlite3_temp_directory = target //nolint:reassign // SQLite exposes this startup-only setting as its process global.
	return nil
}
