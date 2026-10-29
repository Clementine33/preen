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
	DeletedScanSessions      int64
	DeletedGenerations       int64
	DeletedGenerationRecords int64
}

// ==================== Repository ====================

type Repository struct {
	db        *sql.DB
	dbPath    string
	stagingMu sync.Mutex
	staging   map[string]*scanConnection

	// BitrateWriteMu serializes bitrate DB writes across concurrent Analyzer
	// goroutines to avoid SQLITE_BUSY under concurrent folder-plan requests.
	BitrateWriteMu sync.Mutex

	// OnCatalogChanged is called after a commit that changes what the works
	// pages list: work deletion, metadata, inventory merges and observations, renames and
	// library removal or re-rooting. It must be quick and never call back in.
	OnCatalogChanged func()

	// OnMediaFactsNeeded wakes measurement after exact sync commits new or
	// changed files. Like OnCatalogChanged, it must be quick and never call back in.
	OnMediaFactsNeeded func()

	// PlanRetirementGuard serializes plan and record retirement with cover publication.
	// Acquire publication before calling write (which opens its own transaction),
	// then release publication before stopping unneeded downloads. Configure it
	// before workers start; nil is for processes without cover delivery.
	PlanRetirementGuard func(write func() error) error
	// OnPlansRetired wakes bounded plan and record cleanup after retirement or a terminal
	// generation commit that may leave abandoned staging.
	// It must be quick and never call back into the repository.
	OnPlansRetired func()
}

func (r *Repository) catalogChanged() {
	if r.OnCatalogChanged != nil {
		r.OnCatalogChanged()
	}
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
// own the write lock before it reads. Scan TEMP batches explicitly use BEGIN
// DEFERRED and never read main; their merge still uses IMMEDIATE. Read-only
// transactions are unaffected: the driver leaves them deferred.
//
// File-wide auto_vacuum is initialized before the first table instead: setting
// it on every new connection would make even readers wait for the writer lock.
const connectionPragmas = "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate"

func (r *Repository) Close() error {
	r.stagingMu.Lock()
	defer r.stagingMu.Unlock()
	for id, session := range r.staging {
		_ = session.close()
		delete(r.staging, id)
	}
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
