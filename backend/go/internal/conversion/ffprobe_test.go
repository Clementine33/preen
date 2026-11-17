package conversion_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/media"
	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

func TestPlanProbesAACContainers(t *testing.T) {
	for _, ext := range []string{".aac", ".M4A"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			track := filepath.Join(dir, "track"+ext)
			output, err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i",
				"sine=frequency=440:duration=2", "-c:a", "aac", "-b:a", "256k", track).CombinedOutput()
			if err != nil {
				t.Fatalf("generate AAC: %v: %s", err, output)
			}
			track = filepath.ToSlash(track)
			inv := &recordingInventory{observed: []inventory.ObservedAudio{{Path: track}}}
			plan := func() {
				t.Helper()
				if _, planErr := conversion.Plan(
					t.Context(), inv, planSettings{batch: true}, media.ProbeBitrate, mp3Input(filepath.ToSlash(dir)),
				); planErr != nil {
					t.Fatal(planErr)
				}
			}
			plan()
			// Native AAC's measured average need not equal the requested 256k.
			if len(inv.updates) != 1 || inv.updates[0].Path != track || inv.updates[0].BitrateKbps <= 0 {
				t.Fatalf("AAC bitrate: writes=%+v", inv.updates)
			}
			// The inventory now holds the measured rate, and the file is gone: a
			// second pass has nothing to probe and nothing to write.
			inv.observed[0].Bitrate = inv.updates[0].BitrateKbps
			if err := os.Remove(filepath.FromSlash(track)); err != nil {
				t.Fatal(err)
			}
			plan()
			if len(inv.updates) != 1 {
				t.Fatal("cached AAC bitrate was probed and written again")
			}
		})
	}
}

func TestPlanKeepsUnknownAndCachedValues(t *testing.T) {
	dir := t.TempDir()
	invalid := filepath.Join(dir, "invalid.mp3")
	if err := os.WriteFile(invalid, []byte("not audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	cached := filepath.ToSlash(filepath.Join(dir, "cached.mp3"))
	for _, tool := range []string{probe, filepath.Join(dir, "missing-ffprobe")} {
		inv := &recordingInventory{observed: []inventory.ObservedAudio{
			{Path: filepath.ToSlash(invalid)},
			{Path: filepath.ToSlash(filepath.Join(dir, "missing.mp3"))},
			{Path: cached, Bitrate: 192000},
		}}
		var mu sync.Mutex
		var probed []string
		measure := media.New(tool).ProbeBitrate
		counting := func(ctx context.Context, path string) (int64, error) {
			mu.Lock()
			probed = append(probed, path)
			mu.Unlock()
			return measure(ctx, path)
		}
		if _, planErr := conversion.Plan(
			t.Context(), inv, planSettings{batch: true}, counting, mp3Input(filepath.ToSlash(dir)),
		); planErr != nil {
			t.Fatal(planErr)
		}
		// An unmeasurable file leaves its stored rate as it was, so nothing is
		// written; a cached rate is never measured again.
		if len(inv.updates) != 0 || slices.Contains(probed, cached) || len(probed) != 2 {
			t.Fatalf("tool %q: probed %v, writes %+v", tool, probed, inv.updates)
		}
	}
}

func TestPlanReturnsCancellationFromAProbe(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	inv := &recordingInventory{observed: []inventory.ObservedAudio{{Path: "/scope/a.mp3"}}}
	probe := func(context.Context, string) (int64, error) {
		cancel()
		return 0, ctx.Err()
	}
	_, err := conversion.Plan(ctx, inv, planSettings{batch: true}, probe, mp3Input("/scope"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled enrichment: %v", err)
	}
}
