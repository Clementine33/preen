package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// ==================== deployment-wide common settings ====================

const commonSettingsColumns = `operation_type, settings_json, updated_at`

// GetCommonSettings returns the stored deployment-wide common settings of one
// operation kind, or nil when none was ever saved.
func (r *Repository) GetCommonSettings(operationType string) (*workset.CommonSettings, error) {
	var c workset.CommonSettings
	var updatedAt string
	err := r.db.QueryRow(
		`SELECT `+commonSettingsColumns+` FROM common_settings WHERE operation_type = ?`,
		operationType,
	).Scan(&c.OperationType, &c.Settings, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	c.UpdatedAt = parseTimestamp(updatedAt)
	return &c, nil
}

// SyncCommonSettings applies one deployment-wide common-settings save in a
// single transaction (ADR 0008): the stored payload, and the common fields of
// every editable operation of the kind — each with its draft hash and
// operation version.
//
// The current payload (stored, or the task's seed when none was saved) is read
// inside that same transaction and must hash to ExpectedHash. The candidate
// set and each candidate's session state are read there too: a record created
// or a session queued between the caller's read and this write is seen here,
// so the save either covers it or is refused as a whole. A record whose
// library is gone is read-only and never synced (ADR 0001); every generated
// plan is left exactly as it was — revising a plan is planning's job, and a
// rewritten draft is what sends it to needs_planning.
//
// Errors: workset.ErrVersionConflict when the payload moved, and
// *workset.CommonSettingsRefusalError when any candidate holds a queued or
// running session or a document the task cannot read. Either way the
// transaction rolls back: no record is left updated.
func (r *Repository) SyncCommonSettings(p workset.CommonSettingsSync) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin common settings tx: %w", err)
	}
	defer tx.Rollback()

	current := p.SeedSettings
	var stored string
	switch scanErr := tx.QueryRow(
		`SELECT settings_json FROM common_settings WHERE operation_type = ?`, p.OperationType,
	).Scan(&stored); {
	case scanErr == nil:
		current = stored
	case !errors.Is(scanErr, sql.ErrNoRows):
		return scanErr
	}
	if workset.CanonicalJSONHash([]byte(current)) != p.ExpectedHash {
		return workset.ErrVersionConflict
	}

	targets, err := commonSettingsTargetsTx(tx, p.OperationType)
	if err != nil {
		return err
	}
	for _, target := range targets {
		reason, sessionErr := recordActiveSessionTx(tx, target.worksetID)
		if sessionErr != nil {
			return sessionErr
		}
		if reason != "" {
			return refusalOf(target, reason)
		}
		document, hash, rewriteErr := p.Rewrite(target.document)
		if rewriteErr != nil {
			// A stored document the task cannot read is refused, never skipped:
			// a save that updated every other record would be the partial sync
			// this whole write exists to prevent.
			return refusalOf(target, workset.RefusalUnreadableDraft)
		}
		if writeErr := rewriteOperationDraftTx(tx, workset.OperationDraft{
			WorksetID:     target.worksetID,
			OperationType: p.OperationType,
			SchemaVersion: target.schemaVersion,
			DraftJSON:     document,
			DraftHash:     hash,
			UpdatedAt:     p.Now,
		}); writeErr != nil {
			return writeErr
		}
	}

	_, err = tx.Exec(`
		INSERT INTO common_settings (operation_type, settings_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(operation_type) DO UPDATE SET
			settings_json = excluded.settings_json,
			updated_at = excluded.updated_at
	`, p.OperationType, p.Settings, p.Now.Format(timeFormat))
	if err != nil {
		return fmt.Errorf("upsert common settings: %w", err)
	}
	return tx.Commit()
}

// rewriteOperationDraftTx replaces one operation's draft with a rewrite of the
// same save, advancing its version. It is deliberately unguarded: the record
// was enumerated inside this same transaction, which holds the write lock.
func rewriteOperationDraftTx(tx *sql.Tx, d workset.OperationDraft) error {
	if _, err := tx.Exec(`
		UPDATE workset_operations SET version = version + 1, updated_at = ?
		WHERE workset_id = ? AND operation_type = ?
	`, d.UpdatedAt.Format(timeFormat), d.WorksetID, d.OperationType); err != nil {
		return err
	}
	return upsertOperationDraft(tx, d)
}

// refusalOf names one record's refusal of a global common-settings save.
func refusalOf(target commonSettingsTarget, reason string) error {
	return &workset.CommonSettingsRefusalError{
		WorksetID: target.worksetID,
		Title:     target.title,
		Reason:    reason,
	}
}

// commonSettingsTarget is one operation a global save must reach.
type commonSettingsTarget struct {
	worksetID string
	title     string
	// schemaVersion and document are the draft the record holds now; the rewrite
	// keeps the document's own schema version.
	schemaVersion int
	document      string
}

// commonSettingsTargetsTx reads every operation of one kind whose record is
// still editable, with the draft each one holds. A record whose library row is
// gone is orphaned: it stays reviewable and read-only, and a global save never
// writes to it.
func commonSettingsTargetsTx(tx *sql.Tx, operationType string) ([]commonSettingsTarget, error) {
	rows, err := tx.Query(`
		SELECT w.id, w.title, COALESCE(d.schema_version, 1), COALESCE(d.draft_json, '')
		FROM workset_operations o
		JOIN worksets w ON w.id = o.workset_id
		LEFT JOIN workset_operation_drafts d
			ON d.workset_id = o.workset_id AND d.operation_type = o.operation_type
		WHERE o.operation_type = ? AND w.library_id IS NOT NULL AND w.retired = 0
		ORDER BY w.id
	`, operationType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []commonSettingsTarget
	for rows.Next() {
		var target commonSettingsTarget
		if err := rows.Scan(
			&target.worksetID,
			&target.title,
			&target.schemaVersion,
			&target.document,
		); err != nil {
			return nil, err
		}
		out = append(out, target)
	}
	return out, rows.Err()
}
