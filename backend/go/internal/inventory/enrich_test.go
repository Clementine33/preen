package inventory_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/inventory"
)

// fakeProber answers scripted facts and records the paths it was asked for.
type fakeProber struct {
	dims    map[string][2]int
	durs    map[string]float64
	asked   []string
	failAll bool
}

func (p *fakeProber) ImageDimensions(pathPosix string) (int, int, error) {
	p.asked = append(p.asked, pathPosix)
	if p.failAll {
		return 0, 0, errors.New("unmeasurable")
	}
	d, ok := p.dims[pathPosix]
	if !ok {
		return 0, 0, errors.New("unmeasurable")
	}
	return d[0], d[1], nil
}

func (p *fakeProber) AudioDurationSec(_ context.Context, pathPosix string) (float64, error) {
	p.asked = append(p.asked, pathPosix)
	if p.failAll {
		return 0, errors.New("unmeasurable")
	}
	sec, ok := p.durs[pathPosix]
	if !ok {
		return 0, errors.New("unmeasurable")
	}
	return sec, nil
}

// scanWithCandidates runs one root scan whose store reports the given
// candidates after the merge, the seam the enrichment reads through.
func scanWithCandidates(
	t *testing.T, prober inventory.MediaProber, candidates []inventory.MediaCandidate,
) *fakeStagingStore {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "dummy.wav"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := &fakeStagingStore{
		MergeOutcome:    inventory.MergeOutcome{Merged: 1},
		MediaCandidates: candidates,
	}
	pipeline := inventory.NewPipeline(
		store, filesystem.WalkRootEntriesParallel, filesystem.WalkFolderEntries, prober)
	if _, err := pipeline.ScanRoot(root); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestEnrichMediaFactsMeasuresAndWritesBack(t *testing.T) {
	prober := &fakeProber{
		dims: map[string][2]int{"/lib/rj1/cover.png": {1200, 900}},
		durs: map[string]float64{"/lib/rj1/01.mp3": 185.5},
	}
	store := scanWithCandidates(t, prober, []inventory.MediaCandidate{
		{Path: "/lib/rj1/cover.png", Format: "image/png"},
		{Path: "/lib/rj1/01.mp3", Format: "audio/mpeg"},
		{Path: "/lib/rj1/broken.jpg", Format: "image/jpeg"}, // probe fails
		{Path: "/lib/rj1/readme.txt", Format: "text/plain"}, // never a candidate kind
	})

	if len(store.FactsWritten) != 2 {
		t.Fatalf("expected the two measurable facts written, got %v", store.FactsWritten)
	}
	byPath := map[string]inventory.MediaFactsUpdate{}
	for _, u := range store.FactsWritten {
		byPath[u.Path] = u
	}
	if u := byPath["/lib/rj1/cover.png"]; u.Width != 1200 || u.Height != 900 || u.DurationSec != 0 {
		t.Errorf("image fact wrong: %+v", u)
	}
	if u := byPath["/lib/rj1/01.mp3"]; u.DurationSec != 185.5 || u.Width != 0 || u.Height != 0 {
		t.Errorf("audio fact wrong: %+v", u)
	}
	// The unmeasurable and the non-media kind are asked about only when the
	// prober can answer their kind.
	if len(prober.asked) != 3 {
		t.Errorf("expected image, audio and broken image probes, got %v", prober.asked)
	}
}

func TestEnrichMediaFactsSurvivesTotalProbeFailure(t *testing.T) {
	// Every probe failing must still complete the scan: unknown is a value
	// the next scan re-probes, not a failed scan (enrich.go's contract).
	store := scanWithCandidates(t, &fakeProber{failAll: true}, []inventory.MediaCandidate{
		{Path: "/lib/rj1/cover.png", Format: "image/png"},
	})
	if len(store.FactsWritten) != 0 {
		t.Errorf("nothing measurable means nothing written, got %v", store.FactsWritten)
	}
}
