package sqlite

import (
	"fmt"
	"time"

	"github.com/onsei/organizer/backend/internal/inventory"
)

// EntriesMissingMediaFacts pages, in path order after the given path, through
// the inventory's files whose image dimensions or audio duration are still
// unknown (ADR 0015 §2). It spans every root: the background pass resumes
// whatever an earlier pass, or an earlier process, left unmeasured. The format
// filter keeps it to the two kinds a prober can measure; the path key makes
// the cursor an index walk.
func (a *ScanStaging) EntriesMissingMediaFacts(after string, limit int) ([]inventory.MediaCandidate, error) {
	rows, err := a.repo.DB().Query(`
		SELECT path, format FROM entries
		WHERE path > ? AND is_dir = 0 AND format IS NOT NULL
			AND ((format LIKE 'image/%' AND (width IS NULL OR height IS NULL))
				OR (format LIKE 'audio/%' AND duration_sec IS NULL))
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
// table in one transaction. A zero field stores NULL (an image never carries a
// duration, an audio file never dimensions), so one statement shape covers
// both kinds.
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
		SET width = ?, height = ?, duration_sec = ?, updated_at = ?
		WHERE path = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare media-facts update: %w", err)
	}
	defer stmt.Close()

	now := time.Now().Format(timeFormat)
	for _, u := range updates {
		width, height, duration := any(nil), any(nil), any(nil)
		if u.Width > 0 && u.Height > 0 {
			width, height = u.Width, u.Height
		}
		if u.DurationSec > 0 {
			duration = u.DurationSec
		}
		if _, err := stmt.Exec(width, height, duration, now, u.Path); err != nil {
			return fmt.Errorf("update media facts %q: %w", u.Path, err)
		}
	}

	return tx.Commit()
}
