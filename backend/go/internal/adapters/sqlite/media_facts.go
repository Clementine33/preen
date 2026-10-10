package sqlite

import (
	"fmt"
	"time"

	"github.com/onsei/organizer/backend/internal/inventory"
)

// EntriesMissingMediaFacts pages, in path order after the given path, through
// the inventory's files whose image dimensions, audio duration or encoded
// bitrate are still unknown (ADR 0013). The bitrate formats are the
// containers the planner's gate compares against a target. It spans every root: the background pass resumes
// whatever an earlier pass, or an earlier process, left unmeasured. The format
// filter keeps it to the two kinds a prober can measure; the path key makes
// the cursor an index walk.
func (a *ScanStaging) EntriesMissingMediaFacts(after string, limit int) ([]inventory.MediaCandidate, error) {
	rows, err := a.repo.DB().Query(`
		SELECT path, format FROM entries
		WHERE path > ? AND is_dir = 0 AND format IS NOT NULL
			AND ((format LIKE 'image/%' AND (width IS NULL OR height IS NULL))
				OR (format LIKE 'audio/%' AND duration_sec IS NULL)
				OR (format IN ('audio/mpeg', 'audio/mp4', 'audio/aac', 'audio/opus') AND bitrate IS NULL))
		ORDER BY path
		LIMIT ?
	`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("select media-fact candidates: %w", err)
	}
	defer rows.Close()

	var candidates []inventory.MediaCandidate
	for rows.Next() {
		var c inventory.MediaCandidate
		if err := rows.Scan(&c.Path, &c.Format); err != nil {
			return nil, fmt.Errorf("scan media-fact candidate: %w", err)
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// UpdateEntryMediaFacts writes measured media facts back onto the entries
// table in one transaction. A zero field is unmeasured and keeps the stored
// value: a pass that re-measures a file for its missing bitrate never erases a
// duration it failed to read this time. The merge is what clears facts, when
// the file changed.
func (a *ScanStaging) UpdateEntryMediaFacts(updates []inventory.MediaFactsUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	tx, err := a.repo.DB().Begin()
	if err != nil {
		return fmt.Errorf("begin media-facts tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		UPDATE entries
		SET width = COALESCE(?, width), height = COALESCE(?, height),
			duration_sec = COALESCE(?, duration_sec), bitrate = COALESCE(?, bitrate), updated_at = ?
		WHERE path = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare media-facts update: %w", err)
	}
	defer stmt.Close()

	now := time.Now().Format(timeFormat)
	for _, u := range updates {
		width, height, duration, bitrate := any(nil), any(nil), any(nil), any(nil)
		if u.Width > 0 && u.Height > 0 {
			width, height = u.Width, u.Height
		}
		if u.DurationSec > 0 {
			duration = u.DurationSec
		}
		if u.Bitrate > 0 {
			bitrate = u.Bitrate
		}
		if _, err := stmt.Exec(width, height, duration, bitrate, now, u.Path); err != nil {
			return fmt.Errorf("update media facts %q: %w", u.Path, err)
		}
	}

	return tx.Commit()
}
