package conversion

import (
	"context"
	"path"
	"strings"
	"sync"

	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
	"github.com/onsei/organizer/backend/internal/inventory"
)

// ProbeBitrate measures one file's bitrate in bits per second, falling back to
// the ffprobe at ffprobePath (empty: PATH) where the header cannot answer. It
// is the media adapter's measurement - the same one the inventory's background
// pass records (ADR 0013) - so a rate is the same number whichever of the
// two measured it.
type ProbeBitrate func(ctx context.Context, ffprobePath, pathPosix string) (int64, error)

// bitrateAnalyzer measures the MP3/AAC bitrates of a planning root that the
// background pass has not recorded yet, and persists them on the scanned
// entries. It is part of planning: the measured rate is the first entry of the
// encoded-lane gate — a file measuring its target is skipped however it was
// made — so a plan never waits on, or runs without, the background pass.
type bitrateAnalyzer struct {
	inventory   Inventory
	probe       ProbeBitrate
	ffprobePath string
}

func newBitrateAnalyzer(inv Inventory, probe ProbeBitrate, ffprobePath string) *bitrateAnalyzer {
	return &bitrateAnalyzer{inventory: inv, probe: probe, ffprobePath: ffprobePath}
}

// enrichMissing probes the entries whose bitrate is unknown and persists the
// probed values on the entries table.
func (a *bitrateAnalyzer) enrichMissing(ctx context.Context, entries []reconcile.AudioEntry, batchUpdate bool) error {
	idx := selectScopedProbeCandidates(entries)
	if len(idx) == 0 {
		return nil
	}

	workers := min(len(idx), 4)

	jobs := make(chan int)
	var wg sync.WaitGroup
	updates := make([]inventory.BitrateUpdate, 0, len(idx))
	var updatesMu sync.Mutex
	for range workers {
		wg.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				bitrate, err := a.probe(ctx, a.ffprobePath, entries[i].PathPosix)
				if err != nil || bitrate <= 0 {
					continue
				}
				entries[i].Bitrate = bitrate
				updatesMu.Lock()
				updates = append(updates, inventory.BitrateUpdate{Path: entries[i].PathPosix, BitrateKbps: bitrate})
				updatesMu.Unlock()
			}
		})
	}

	for _, i := range idx {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}

	// Nothing probed means nothing to write: a root whose files all still fail
	// to probe leaves its stored rates as they were.
	if len(updates) == 0 {
		return nil
	}

	// The write shape (per row, or chunked in one transaction) and its retry
	// on a locked database belong to the adapter that owns the table.
	return a.inventory.UpdateEntryBitrates(updates, batchUpdate)
}

// selectScopedProbeCandidates names the encoded containers whose bitrate is
// worth measuring: the targets the planner compares a stored file
// against.
func selectScopedProbeCandidates(entries []reconcile.AudioEntry) []int {
	idx := make([]int, 0, len(entries))
	for i := range entries {
		if entries[i].Bitrate > 0 {
			continue
		}
		switch strings.ToLower(path.Ext(entries[i].PathPosix)) {
		case ".mp3", ".aac", ".m4a", ".opus":
			idx = append(idx, i)
		}
	}
	return idx
}
