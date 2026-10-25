package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/Clementine33/preen/backend/internal/inventory"
)

// ScanStaging writes each scan into its private TEMP database and tracks its lifecycle.
type ScanStaging struct {
	repo *Repository
	// testExecSeam allows tests to inject failures after N exec calls.
	// Production code leaves this nil (uses real exec).
	testExecSeam func(callCount int) error
}

// NewScanStaging creates the scan-session storage adapter.
func NewScanStaging(repo *Repository) *ScanStaging {
	return &ScanStaging{repo: repo}
}

// execFn is a testable wrapper around sql.Stmt.Exec for deterministic failure injection.
func (a *ScanStaging) execStmt(stmt *sql.Stmt, callCount int, args ...any) (sql.Result, error) {
	if a.testExecSeam != nil {
		if err := a.testExecSeam(callCount); err != nil {
			return nil, err
		}
	}
	return stmt.Exec(args...)
}

const defaultBatchSize = 1000

// WriteStagingEntries commits one TEMP-only batch without acquiring the main writer lock.
func (a *ScanStaging) WriteStagingEntries(sessionID string, entries []inventory.StagingEntry) error {
	if len(entries) == 0 {
		return nil
	}
	root := entries[0].RootPath
	session, err := a.repo.scanConnection(sessionID, root)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if _, err = session.conn.ExecContext(ctx, "BEGIN DEFERRED"); err != nil {
		return err
	}
	defer func() { _, _ = session.conn.ExecContext(ctx, "ROLLBACK") }()
	stmt, err := session.conn.PrepareContext(ctx, `INSERT OR REPLACE INTO temp.entries_staging
 (path,parent_path,name,is_dir,is_work_root,size,mtime,format) VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, e := range entries {
		if e.RootPath != "" && filepath.ToSlash(e.RootPath) != session.root {
			return fmt.Errorf("staging root mismatch")
		}
		if _, err = a.execStmt(
			stmt,
			i+1,
			e.Path,
			e.ParentPath,
			e.Name,
			e.IsDir,
			e.IsWorkRoot,
			e.Size,
			e.Mtime,
			e.Format,
		); err != nil {
			return fmt.Errorf("insert staging entry %q: %w", e.Path, err)
		}
	}
	_, err = session.conn.ExecContext(ctx, "COMMIT")
	return err
}

// MergeStaging merges staged entries and reconciles the library's bindings,
// reporting what the merge did. The count and the classifications are produced
// by the merge itself, inside its transaction, so nothing here recomputes them
// from a state that has already moved on.
func (a *ScanStaging) MergeStaging(sessionID, libraryID, rootPath string) (inventory.MergeOutcome, error) {
	return a.repo.MergeStaging(sessionID, libraryID, rootPath)
}

// CreateScanSession persists a scanner scan session in sqlite.
func (a *ScanStaging) CreateScanSession(session *inventory.ScanSession) error {
	var scopePath *string
	if session.ScopePath != nil {
		p := filepath.ToSlash(*session.ScopePath)
		scopePath = &p
	}

	err := a.repo.CreateScanSession(&inventory.ScanSession{
		SessionID:    session.SessionID,
		RootPath:     filepath.ToSlash(session.RootPath),
		ScopePath:    scopePath,
		Kind:         session.Kind,
		Status:       session.Status,
		ErrorCode:    session.ErrorCode,
		ErrorMessage: session.ErrorMessage,
		StartedAt:    session.StartedAt,
		FinishedAt:   session.FinishedAt,
	})
	if err != nil {
		return err
	}
	_, err = a.repo.scanConnection(session.SessionID, filepath.ToSlash(session.RootPath))
	if err != nil {
		_ = a.repo.UpdateScanSessionStatus(session.SessionID, "failed", "STAGING_OPEN_FAILED", err.Error())
	}
	return err
}

// UpdateScanSessionStatus updates session lifecycle status in sqlite.
func (a *ScanStaging) UpdateScanSessionStatus(sessionID, status, errorCode, errorMessage string) error {
	return a.repo.UpdateScanSessionStatus(sessionID, status, errorCode, errorMessage)
}

// CloseStagingSession releases the physical connection and its TEMP database.
func (a *ScanStaging) CloseStagingSession(sessionID string) error {
	return a.repo.closeStagingSession(sessionID)
}

// WriteStagingBatch commits each batch independently in the scan's TEMP database.
func (a *ScanStaging) WriteStagingBatch(sessionID string, batch []inventory.StagingEntry) error {
	return a.WriteStagingEntries(sessionID, batch)
}

var _ inventory.StagingStore = (*ScanStaging)(nil)
