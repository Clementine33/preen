package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
)

// scanConnection owns one physical connection from the first TEMP write through merge.
// Closing the whole private handle prevents database/sql from pooling its TEMP database.
type scanConnection struct {
	db   *sql.DB
	conn *sql.Conn
	root string
}

func (s *scanConnection) close() error { return errors.Join(s.conn.Close(), s.db.Close()) }

func (r *Repository) scanConnection(id, root string) (*scanConnection, error) {
	r.stagingMu.Lock()
	defer r.stagingMu.Unlock()
	if s := r.staging[id]; s != nil {
		return s, nil
	}
	if r.dbPath == ":memory:" {
		return nil, fmt.Errorf("scan staging requires a file database")
	}
	db, err := sql.Open("sqlite", r.dbPath+connectionPragmas)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &scanConnection{db: db, conn: conn, root: filepath.ToSlash(root)}
	if _, err = conn.ExecContext(context.Background(), `CREATE TEMP TABLE entries_staging (
 path TEXT PRIMARY KEY, parent_path TEXT NOT NULL, name TEXT NOT NULL,
 is_dir INTEGER NOT NULL, is_work_root INTEGER NOT NULL DEFAULT 0,
 size INTEGER NOT NULL, mtime INTEGER NOT NULL, format TEXT NOT NULL
 ) WITHOUT ROWID`); err != nil {
		_ = s.close()
		return nil, err
	}
	r.staging[id] = s
	return s, nil
}

func (r *Repository) closeStagingSession(id string) error {
	r.stagingMu.Lock()
	defer r.stagingMu.Unlock()
	if s := r.staging[id]; s != nil {
		delete(r.staging, id)
		return s.close()
	}
	return nil
}
