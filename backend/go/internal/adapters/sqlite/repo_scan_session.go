package sqlite

import (
	"database/sql"
	"time"

	"github.com/Clementine33/preen/backend/internal/inventory"
)

// ==================== Scan Session Methods ====================

// CreateScanSession validates root ownership and creates a session in one transaction.
func (r *Repository) CreateScanSession(s *inventory.ScanSession) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = checkScanRoot(tx, s.RootPath); err != nil {
		return err
	}

	var scopePath any
	if s.ScopePath != nil {
		scopePath = *s.ScopePath
	}
	_, err = tx.Exec(`
		INSERT INTO scan_sessions (session_id, root_path, scope_path, kind, status, started_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, s.SessionID, s.RootPath, scopePath, s.Kind, s.Status, s.StartedAt.Format(timeFormat))
	if err != nil {
		return err
	}
	return tx.Commit()
}

// GetScanSession retrieves a scan session by ID.
func (r *Repository) GetScanSession(sessionID string) (*inventory.ScanSession, error) {
	var s inventory.ScanSession
	var startedAtStr string
	var finishedAtStr, errorCode, errorMessage, scopePath sql.NullString
	err := r.db.QueryRow(`
		SELECT session_id, root_path, scope_path, kind, status, error_code, error_message, started_at, finished_at
		FROM scan_sessions WHERE session_id = ?
	`, sessionID).Scan(&s.SessionID, &s.RootPath, &scopePath, &s.Kind, &s.Status, &errorCode, &errorMessage, &startedAtStr, &finishedAtStr)
	if err != nil {
		return nil, err
	}
	if scopePath.Valid {
		s.ScopePath = &scopePath.String
	}
	s.ErrorCode = errorCode.String
	s.ErrorMessage = errorMessage.String
	s.StartedAt = parseTimestamp(startedAtStr)
	if finishedAtStr.Valid && finishedAtStr.String != "" {
		s.FinishedAt = parseTimestamp(finishedAtStr.String)
	}
	return &s, nil
}

// UpdateScanSessionStatus updates scan session status.
func (r *Repository) UpdateScanSessionStatus(sessionID, status, errorCode, errorMessage string) error {
	finishedAt := "NULL"
	if status == "completed" || status == "failed" || status == "canceled" {
		finishedAt = "datetime('now')"
	}
	if finishedAt == "NULL" {
		_, err := r.db.Exec(`
			UPDATE scan_sessions SET status = ?, error_code = ?, error_message = ? WHERE session_id = ?
		`, status, errorCode, errorMessage, sessionID)
		return err
	}
	_, err := r.db.Exec(`
		UPDATE scan_sessions SET status = ?, error_code = ?, error_message = ?, finished_at = datetime('now') WHERE session_id = ?
	`, status, errorCode, errorMessage, sessionID)
	return err
}

// InterruptStaleScanSessions marks queued/running/merging scan sessions
// interrupted. Scans run in-process and only ever write those three states
// while they are running, so a non-terminal row found at startup belongs to a
// process that died without finalizing it.
//
// This is what makes retention's terminal-only rule safe. Without it a crashed
// row would stay non-terminal forever: never eligible for cleanup, and
// HasActiveScanForRoot would keep refusing planning and execution against its
// root.
//
// No scan runs yet at startup, so every staging row belongs to a dead session,
// including ones interrupted or deleted earlier; all of them are dropped.
func (r *Repository) InterruptStaleScanSessions() (int64, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`
		UPDATE scan_sessions SET status = 'interrupted', error_code = 'INTERRUPTED', finished_at = ?
		WHERE status IN ('queued','running','merging')
	`, time.Now().Format(timeFormat))
	if err != nil {
		return 0, err
	}
	if _, clearErr := tx.Exec("DELETE FROM entries_staging"); clearErr != nil {
		return 0, clearErr
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// ListScanSessionsByRoot returns scan sessions for a root.
func (r *Repository) ListScanSessionsByRoot(rootPath string) ([]*inventory.ScanSession, error) {
	rows, err := r.db.Query(`
		SELECT session_id, root_path, scope_path, kind, status, started_at, finished_at
		FROM scan_sessions WHERE root_path = ? ORDER BY started_at DESC
	`, rootPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*inventory.ScanSession
	for rows.Next() {
		var s inventory.ScanSession
		var startedAtStr string
		var finishedAtStr, scopePath sql.NullString
		if err := rows.Scan(
			&s.SessionID,
			&s.RootPath,
			&scopePath,
			&s.Kind,
			&s.Status,
			&startedAtStr,
			&finishedAtStr,
		); err != nil {
			return nil, err
		}
		if scopePath.Valid {
			s.ScopePath = &scopePath.String
		}
		s.StartedAt = parseTimestamp(startedAtStr)
		if finishedAtStr.Valid && finishedAtStr.String != "" {
			s.FinishedAt = parseTimestamp(finishedAtStr.String)
		}
		sessions = append(sessions, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}
