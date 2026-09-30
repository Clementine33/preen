package inventory

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
)

// mediaProbeWorkers bounds the concurrent probes of one enrichment pass, the
// same width the bitrate analyzer uses.
const mediaProbeWorkers = 4

// enrichMediaFacts probes the entries this scan touched whose image dimensions
// or audio duration are still unknown, and writes the measured facts back
// (ADR 0015 §2). It runs after the merge, inside the scan's admission slot.
//
// The whole pass is best-effort and deliberately cannot fail the scan: the
// merge is already committed, so reporting failure would misreport an updated
// inventory, and a fact left NULL is simply re-probed by the next scan that
// touches the entry. A prober that is not wired (nil) skips the pass, which is
// the state every pipeline test runs in.
func (s *Pipeline) enrichMediaFacts(ctx context.Context, sessionID string) {
	if s.prober == nil {
		return
	}
	candidates, err := s.repo.EntriesMissingMediaFacts(sessionID)
	if err != nil || len(candidates) == 0 {
		return
	}

	var updates []MediaFactsUpdate
	var mu sync.Mutex
	var next atomic.Int64
	var wg sync.WaitGroup
	for range min(mediaProbeWorkers, len(candidates)) {
		wg.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= len(candidates) || ctx.Err() != nil {
					return
				}
				c := candidates[i]
				u := MediaFactsUpdate{Path: c.Path}
				var err error
				switch {
				case strings.HasPrefix(c.Format, "image/"):
					u.Width, u.Height, err = s.prober.ImageDimensions(c.Path)
				case strings.HasPrefix(c.Format, "audio/"):
					u.DurationSec, err = s.prober.AudioDurationSec(ctx, c.Path)
				default:
					continue
				}
				if err != nil || (u.Width <= 0 && u.DurationSec <= 0) {
					continue // unmeasurable stays NULL, not zero
				}
				mu.Lock()
				updates = append(updates, u)
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if len(updates) > 0 {
		_ = s.repo.UpdateEntryMediaFacts(updates)
	}
}
