package sqlite

import (
	"database/sql"
	"os"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// timeFormat is the format used for storing timestamps in SQLite.
const timeFormat = time.RFC3339Nano

// parseTimestamp parses a timestamp string with fallback to SQLite's default format.
func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Try RFC3339Nano first (our preferred format)
	if t, err := time.Parse(timeFormat, s); err == nil {
		return t
	}
	// Fallback to SQLite's default format
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t
	}
	// Fallback to SQLite's datetime() function format
	if t, err := time.Parse("2006-01-02 15:04:05.999999999", s); err == nil {
		return t
	}
	return time.Time{}
}

// CleanupStats holds counts of rows deleted by each cleanup operation.
type CleanupStats struct {
	DeletedScanSessions int64
	DeletedGenerations  int64
}

// ==================== Repository ====================

type Repository struct {
	db *sql.DB

	// BitrateWriteMu serializes bitrate DB writes across concurrent Analyzer
	// goroutines to avoid SQLITE_BUSY under concurrent folder-plan requests.
	BitrateWriteMu sync.Mutex
}

// connectionPragmas is appended to the database path to configure every
// connection the driver opens. Foreign keys and busy_timeout are connection
// scoped, so one-shot PRAGMA statements would miss other pooled connections.
//
// txlock=immediate starts every read-write transaction by taking the write
// lock. Deferred transactions take their read snapshot at the first statement
// and then upgrade it when they write; if another connection commits in
// between, that upgrade is refused (SQLITE_BUSY_SNAPSHOT) rather than applied -
// and SQLite only started refusing it in 3.51.3, where losing the race used to
// be silent. Every write path here reads before it writes (find the queued
// session, load the draft, then claim or persist), so the transaction has to
// own the write lock before it reads. Read-only transactions are unaffected:
// the driver leaves them deferred.
//
// auto_vacuum(2) is incremental auto-vacuum, which only takes effect before the
// first table exists: the connection that creates the file writes the mode into
// the header, and every connection after that reads it back from there. On a
// database that already has tables the setting is a silent no-op, which is why
// reclamation asks AutoVacuumMode what the file was created with instead of
// trying to enable it later.
const connectionPragmas = "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=auto_vacuum(2)&_txlock=immediate"

func (r *Repository) Close() error {
	return r.db.Close()
}

// DB returns the underlying database connection.
func (r *Repository) DB() *sql.DB {
	return r.db
}

// EnsureDBPath creates the database file at given path.
func EnsureDBPath(path string) error {
	dir := path
	for len(dir) > 0 && dir[len(dir)-1] != '/' && dir[len(dir)-1] != '\\' {
		dir = dir[:len(dir)-1]
	}
	if dir != "" {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}
