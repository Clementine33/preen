package conversion_test

import (
	"sync"

	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

// planSettings is the planner's settings port with only the batch option set.
type planSettings struct{ batch bool }

func (s planSettings) Plan() conversion.PlanConfig {
	return conversion.PlanConfig{BatchUpdate: s.batch}
}
func (planSettings) PruneNoEffectTags() []string { return nil }
func (planSettings) PruneOtherTags() []string    { return nil }

// mp3Input plans one root toward MP3@320, the profile whose gate reads a
// file's bitrate first.
func mp3Input(root string) conversion.Input {
	profile := reconcile.DesiredProfile{Encoded: &reconcile.AudioOutputSpec{
		Codec: reconcile.CodecMp3, Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 320},
	}}
	policy := reconcile.Policy{
		SchemaVersion: 1, Mode: reconcile.ModeAvailableSources,
		NoEffectTags: []string{"SEなし"}, Matched: profile, Unmatched: profile,
	}
	return conversion.Input{Policy: policy, Roots: []conversion.RootInput{{Path: root, Policy: policy}}}
}

// recordingInventory stands in for the inventory port in the planner's own
// tests: it remembers what it was handed and answers with whatever the test set
// up. The write shape behind the port — per row, or chunked into one
// transaction — belongs to the adapter that owns the table, and is tested there.
type recordingInventory struct {
	mu       sync.Mutex
	updates  []inventory.BitrateUpdate
	batch    []bool
	writeErr error
	observed []inventory.ObservedAudio
	readErr  error
}

func (r *recordingInventory) ObservedAudioEntries(string) ([]inventory.ObservedAudio, error) {
	return r.observed, r.readErr
}

func (r *recordingInventory) ObservedAudioEntriesByRoot(roots []string) (map[string][]inventory.ObservedAudio, error) {
	out := map[string][]inventory.ObservedAudio{}
	for _, root := range roots {
		out[root] = r.observed
	}
	return out, r.readErr
}

func (r *recordingInventory) RootExistsInInventory(string) (bool, error) { return true, nil }

func (r *recordingInventory) UpdateEntryBitrates(updates []inventory.BitrateUpdate, batch bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, updates...)
	r.batch = append(r.batch, batch)
	return r.writeErr
}

// written returns the paths the writer was handed, in write order.
func (r *recordingInventory) written() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	paths := make([]string, 0, len(r.updates))
	for _, u := range r.updates {
		paths = append(paths, u.Path)
	}
	return paths
}
