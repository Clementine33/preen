package sqlite //nolint:testpackage // white-box tests share the package's own helpers

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/onsei/organizer/backend/internal/inventory"
)

// stageFactsRow seeds one staging row. A merge clears its session's staging
// (scan_merge.go step 4), so a session's rows are all staged before the one
// merge that consumes them — staging row by row and merging each would have
// the full-root stale cleanup eat the earlier rows.
func stageFactsRow(t *testing.T, repo *Repository, root, path, format string, size, mtime int64, sessionID string) {
	t.Helper()
	_, err := repo.db.Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, size, mtime, format, operation)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, 'upsert')
	`, sessionID, path, root, root, path, size, mtime, format)
	if err != nil {
		t.Fatalf("seed staging: %v", err)
	}
}

// mergeFactsFixture stages one entry and merges its session in one step.
func mergeFactsFixture(t *testing.T, repo *Repository, root, path, format string, size, mtime int64, sessionID string) {
	t.Helper()
	stageFactsRow(t, repo, root, path, format, size, mtime, sessionID)
	if err := repo.MergeStagingSimple(sessionID, root); err != nil {
		t.Fatalf("merge: %v", err)
	}
}

func TestMergePreservesMediaFactsWhenUnchanged(t *testing.T) {
	repo, err := NewRepository(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()
	mergeFactsFixture(t, repo, "/music", "/music/cover.png", "image/png", 1024, now, "scan-1")

	adapter := NewScanStaging(repo)
	if err := adapter.UpdateEntryMediaFacts([]inventory.MediaFactsUpdate{
		{Path: "/music/cover.png", Width: 1200, Height: 900},
	}); err != nil {
		t.Fatalf("update media facts: %v", err)
	}

	// The same file, scanned again: size and mtime unchanged, facts preserved.
	mergeFactsFixture(t, repo, "/music", "/music/cover.png", "image/png", 1024, now, "scan-2")

	var width, height sql.NullInt64
	if err := repo.db.QueryRow("SELECT width, height FROM entries WHERE path = ?", "/music/cover.png").
		Scan(&width, &height); err != nil {
		t.Fatal(err)
	}
	if !width.Valid || width.Int64 != 1200 || !height.Valid || height.Int64 != 900 {
		t.Errorf("expected preserved 1200x900, got %v x %v", width.Int64, height.Int64)
	}

	// A changed file clears them: the recorded facts describe old bytes.
	mergeFactsFixture(t, repo, "/music", "/music/cover.png", "image/png", 2048, now, "scan-3")
	if err := repo.db.QueryRow("SELECT width, height FROM entries WHERE path = ?", "/music/cover.png").
		Scan(&width, &height); err != nil {
		t.Fatal(err)
	}
	if width.Valid || height.Valid {
		t.Errorf("expected cleared facts after change, got %v x %v", width.Int64, height.Int64)
	}
}

func TestEntriesMissingMediaFactsAndWriteBack(t *testing.T) {
	repo, err := NewRepository(":memory:")
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()
	stageFactsRow(t, repo, "/music", "/music/cover.png", "image/png", 10, now, "scan-1")
	stageFactsRow(t, repo, "/music", "/music/track.mp3", "audio/mpeg", 20, now, "scan-1")
	stageFactsRow(t, repo, "/music", "/music/readme.txt", "text/plain", 30, now, "scan-1")
	if mergeErr := repo.MergeStagingSimple("scan-1", "/music"); mergeErr != nil {
		t.Fatal(mergeErr)
	}
	// Another root's row qualifies too: the pass spans the whole inventory.
	mergeFactsFixture(t, repo, "/other", "/other/other.wav", "audio/wav", 40, now, "scan-other")

	adapter := NewScanStaging(repo)
	candidates, err := adapter.EntriesMissingMediaFacts("", 10)
	if err != nil {
		t.Fatalf("select candidates: %v", err)
	}
	if got := candidatePaths(candidates); got != "/music/cover.png /music/track.mp3 /other/other.wav" {
		t.Fatalf("expected every unmeasured media file in path order, got %q", got)
	}
	// The cursor pages: after a path, at most limit rows.
	page, err := adapter.EntriesMissingMediaFacts("/music/cover.png", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := candidatePaths(page); got != "/music/track.mp3" {
		t.Errorf("expected the next candidate only, got %q", got)
	}

	// The image is measured; the audio probe fails, which the pass records by
	// leaving it alone — unknown is a value, not an error.
	if updateErr := adapter.UpdateEntryMediaFacts([]inventory.MediaFactsUpdate{
		{Path: "/music/cover.png", Width: 800, Height: 600},
	}); updateErr != nil {
		t.Fatalf("update media facts: %v", updateErr)
	}

	remaining, err := adapter.EntriesMissingMediaFacts("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := candidatePaths(remaining); got != "/music/track.mp3 /other/other.wav" {
		t.Errorf("expected only the unmeasured audio to remain, got %q", got)
	}

	// A zero duration stores NULL rather than 0: unmeasured is not zero
	// seconds. Each row writes the whole fact set of its kind — the
	// enrichment never mixes dimensions and duration in one row.
	if updateErr := adapter.UpdateEntryMediaFacts([]inventory.MediaFactsUpdate{
		{Path: "/music/track.mp3", DurationSec: 185.5},
	}); updateErr != nil {
		t.Fatal(updateErr)
	}
	var duration sql.NullFloat64
	if err := repo.db.QueryRow("SELECT duration_sec FROM entries WHERE path = ?", "/music/track.mp3").
		Scan(&duration); err != nil {
		t.Fatal(err)
	}
	if !duration.Valid || duration.Float64 != 185.5 {
		t.Errorf("expected duration 185.5, got %v", duration.Float64)
	}
}

func candidatePaths(candidates []inventory.MediaCandidate) string {
	paths := make([]string, len(candidates))
	for i, c := range candidates {
		paths[i] = c.Path
	}
	return strings.Join(paths, " ")
}

func TestEncodedAudioWaitsForItsBitrateAndKeepsKnownFacts(t *testing.T) {
	repo, err := NewRepository(":memory:")
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	defer repo.Close()

	now := time.Now().Unix()
	stageFactsRow(t, repo, "/music", "/music/a.mp3", "audio/mpeg", 10, now, "scan-1")
	stageFactsRow(t, repo, "/music", "/music/b.wav", "audio/wav", 20, now, "scan-1")
	if err = repo.MergeStagingSimple("scan-1", "/music"); err != nil {
		t.Fatal(err)
	}
	adapter := NewScanStaging(repo)
	if err = adapter.UpdateEntryMediaFacts([]inventory.MediaFactsUpdate{
		{Path: "/music/a.mp3", DurationSec: 60},
		{Path: "/music/b.wav", DurationSec: 90},
	}); err != nil {
		t.Fatal(err)
	}

	// The MP3 still lacks the bitrate the planner compares; a WAV carries none.
	candidates, err := adapter.EntriesMissingMediaFacts("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := candidatePaths(candidates); got != "/music/a.mp3" {
		t.Fatalf("expected only the MP3 awaiting its bitrate, got %q", got)
	}

	// A pass that measures only the bitrate keeps the duration it already had.
	if err := adapter.UpdateEntryMediaFacts([]inventory.MediaFactsUpdate{
		{Path: "/music/a.mp3", Bitrate: 320000},
	}); err != nil {
		t.Fatal(err)
	}
	var duration sql.NullFloat64
	var bitrate sql.NullInt64
	if err := repo.db.QueryRow("SELECT duration_sec, bitrate FROM entries WHERE path = ?", "/music/a.mp3").
		Scan(&duration, &bitrate); err != nil {
		t.Fatal(err)
	}
	if duration.Float64 != 60 || bitrate.Int64 != 320000 {
		t.Errorf("facts = %v s, %v bps; want 60 s, 320000 bps", duration.Float64, bitrate.Int64)
	}
	if candidates, _ := adapter.EntriesMissingMediaFacts("", 10); len(candidates) != 0 {
		t.Errorf("nothing should remain, got %q", candidatePaths(candidates))
	}
}
