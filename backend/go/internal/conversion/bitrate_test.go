package conversion_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/media"
	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

var mp3Fixture = sync.OnceValues(func() ([]byte, error) {
	return exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-c:a", "libmp3lame", "-b:a", "128k", "-f", "mp3", "pipe:1").Output()
})

func writeTestMP3Frame(t *testing.T, path string) {
	t.Helper()
	data, err := mp3Fixture()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPlanProbesOnlyScopedMissingMP3AndAAC pins which observed rows the planner
// measures: encoded containers whose stored bitrate is unknown, by extension and
// regardless of case.
func TestPlanProbesOnlyScopedMissingMP3AndAAC(t *testing.T) {
	inv := &recordingInventory{observed: []inventory.ObservedAudio{
		{Path: "/scope/a.mp3"},
		{Path: "/scope/b.mp3", Bitrate: 128000},
		{Path: "/scope/c.flac"},
		{Path: "/scope/d.MP3"},
		{Path: "/scope/e.aac"},
		{Path: "/scope/f.M4A"},
		{Path: "/scope/g.m4a", Bitrate: 256000},
	}}
	var mu sync.Mutex
	var probed []string
	probe := func(_ context.Context, path string) (int64, error) {
		mu.Lock()
		defer mu.Unlock()
		probed = append(probed, path)
		return 0, nil
	}
	if _, err := conversion.Plan(t.Context(), inv, planSettings{batch: true}, probe, mp3Input("/scope")); err != nil {
		t.Fatal(err)
	}
	slices.Sort(probed)
	want := []string{"/scope/a.mp3", "/scope/d.MP3", "/scope/e.aac", "/scope/f.M4A"}
	if !slices.Equal(probed, want) {
		t.Fatalf("probed %v, want %v", probed, want)
	}
}

// TestPlanHandsOnlyScopedEntriesToTheInventory pins what the planner persists:
// exactly the entries the inventory reported and the probe measured, each with
// the rate it measured, in one call carrying the batch option through.
func TestPlanHandsOnlyScopedEntriesToTheInventory(t *testing.T) {
	tmpDir := t.TempDir()

	const scopedTotal = 120
	observed := make([]inventory.ObservedAudio, 0, scopedTotal)
	wantPaths := make([]string, 0, scopedTotal)
	for i := range scopedTotal {
		p := filepath.Join(tmpDir, fmt.Sprintf("in-scope-%03d.mp3", i))
		writeTestMP3Frame(t, p)
		pPosix := filepath.ToSlash(p)
		observed = append(observed, inventory.ObservedAudio{Path: pPosix, Format: "audio/mpeg"})
		wantPaths = append(wantPaths, pPosix)
	}
	// A track this root never collected: it must never be written, whatever its
	// extension says, because the planner only persists what the inventory handed it.
	outOfScopePath := filepath.ToSlash(filepath.Join(tmpDir, "out-of-scope.mp3"))
	writeTestMP3Frame(t, filepath.FromSlash(outOfScopePath))

	inv := &recordingInventory{observed: observed}
	if _, err := conversion.Plan(
		t.Context(), inv, planSettings{batch: true}, media.ProbeBitrate, mp3Input(filepath.ToSlash(tmpDir)),
	); err != nil {
		t.Fatalf("plan scoped entries: %v", err)
	}

	written := inv.written()
	if len(written) != scopedTotal {
		t.Fatalf("wrote %d entries, want %d", len(written), scopedTotal)
	}
	// Probing runs concurrently, so the writes arrive in completion order:
	// what is pinned is which paths were written, each exactly once.
	seen := make(map[string]int, len(written))
	for _, path := range written {
		seen[path]++
	}
	for _, path := range wantPaths {
		if seen[path] != 1 {
			t.Fatalf("path %q written %d times, want once", path, seen[path])
		}
	}
	if seen[outOfScopePath] != 0 {
		t.Fatalf("wrote %q, which the inventory never handed to the planner", outOfScopePath)
	}
	for _, u := range inv.updates {
		if u.BitrateKbps <= 0 {
			t.Fatalf("%q was handed to the writer without its probed rate", u.Path)
		}
	}
	if len(inv.batch) != 1 || !inv.batch[0] {
		t.Fatalf("batch option reached the writer as %v, want one call with true", inv.batch)
	}
}

// TestPlanReturnsPersistError keeps a failing write visible: the planner
// must not report a successful pass over facts that were not stored.
func TestPlanReturnsPersistError(t *testing.T) {
	tmpDir := t.TempDir()
	mp3Path := filepath.Join(tmpDir, "track.mp3")
	writeTestMP3Frame(t, mp3Path)

	writeErr := errors.New("write failed")
	inv := &recordingInventory{
		writeErr: writeErr,
		observed: []inventory.ObservedAudio{{Path: filepath.ToSlash(mp3Path), Format: "audio/mpeg"}},
	}
	_, err := conversion.Plan(
		t.Context(), inv, planSettings{batch: true}, media.ProbeBitrate, mp3Input(filepath.ToSlash(tmpDir)),
	)
	if !errors.Is(err, writeErr) {
		t.Fatalf("plan error = %v, want the writer's failure", err)
	}
}

func TestPlanWithBatchOption_DoesNotEmitGlobalLogMetrics(t *testing.T) {
	var buf bytes.Buffer
	oldOut := log.Writer()
	oldFlags := log.Flags()
	oldPrefix := log.Prefix()
	log.SetOutput(&buf)
	log.SetFlags(0)
	log.SetPrefix("")
	defer func() {
		log.SetOutput(oldOut)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	}()

	inv := &recordingInventory{observed: []inventory.ObservedAudio{{Path: "/scope/a.flac", Format: "audio/flac"}}}
	probe := func(context.Context, string) (int64, error) {
		t.Error("a flac entry must not be probed")
		return 0, nil
	}
	if _, err := conversion.Plan(t.Context(), inv, planSettings{batch: true}, probe, mp3Input("/scope")); err != nil {
		t.Fatalf("expected plan success, got %v", err)
	}

	if got := strings.TrimSpace(buf.String()); got != "" {
		t.Fatalf("expected no global log output, got %q", got)
	}
}
