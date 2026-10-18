package sqlite

import (
	"fmt"
	"runtime"
	"time"

	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/library"
	"github.com/Clementine33/preen/backend/internal/librarypath"
)

// missingMediaFacts is the candidate predicate shared by the pass and its
// counts, so what the status reports is exactly what the pass still visits.
// Images are the formats the prober has a header decoder for (media/probe.go);
// any other image keeps unknown dimensions without being retried.
const missingMediaFacts = `is_dir = 0 AND format IS NOT NULL
	AND ((format IN ('image/jpeg', 'image/png', 'image/gif', 'image/webp') AND (width IS NULL OR height IS NULL))
		OR (format LIKE 'audio/%' AND duration_sec IS NULL)
		OR (format IN ('audio/mpeg', 'audio/mp4', 'audio/aac', 'audio/opus') AND bitrate IS NULL))`

// EntriesMissingMediaFacts pages, in path order after the given path, through
// the inventory's files whose image dimensions, audio duration or encoded
// bitrate are still unknown (ADR 0013). The bitrate formats are the
// containers the planner's gate compares against a target. It spans every root: the background pass resumes
// whatever an earlier pass, or an earlier process, left unmeasured. The format
// filter keeps it to the two kinds a prober can measure; the path key makes
// the cursor an index walk.
func (a *ScanStaging) EntriesMissingMediaFacts(after string, limit int) ([]inventory.MediaCandidate, error) {
	libraries, err := a.repo.ListLibraries()
	if err != nil {
		return nil, fmt.Errorf("load media storage classes: %w", err)
	}
	// A spelling-only root edit keeps inventory paths, so match canonical identities.
	classes := make(map[string]library.StorageClass, len(libraries))
	for _, lib := range libraries {
		classes[librarypath.RootPathKey(lib.RootPath)] = lib.StorageClass
	}
	rows, err := a.repo.DB().Query(`
		SELECT path, format, root_path FROM entries
		WHERE path > ? AND `+missingMediaFacts+`
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
		var root string
		if err := rows.Scan(&c.Path, &c.Format, &root); err != nil {
			return nil, fmt.Errorf("scan media-fact candidate: %w", err)
		}
		c.ProbeWorkers = classes[librarypath.RootPathKey(root)].MediaWorkers(runtime.NumCPU())
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// MediaFactsCounts counts, per library, the files still missing media facts,
// split at the pass cursor: those after it are pending, those up to it were
// visited and stay unknown. Entries belong to a library by the root they were
// scanned under, so no path pattern is involved. Libraries with nothing
// missing are absent.
func (a *ScanStaging) MediaFactsCounts(after string) ([]inventory.LibraryMediaCounts, error) {
	rows, err := a.repo.DB().Query(`
		SELECT l.id, COALESCE(SUM(e.path > ?1), 0), COALESCE(SUM(e.path <= ?1), 0)
		FROM entries e JOIN libraries l ON l.root_path = e.root_path
		WHERE `+missingMediaFacts+`
		GROUP BY l.id
		ORDER BY l.id
	`, after)
	if err != nil {
		return nil, fmt.Errorf("count media-fact candidates: %w", err)
	}
	defer rows.Close()

	var counts []inventory.LibraryMediaCounts
	for rows.Next() {
		var c inventory.LibraryMediaCounts
		if err := rows.Scan(&c.LibraryID, &c.Pending, &c.Unmeasurable); err != nil {
			return nil, fmt.Errorf("scan media-fact counts: %w", err)
		}
		counts = append(counts, c)
	}
	return counts, rows.Err()
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
