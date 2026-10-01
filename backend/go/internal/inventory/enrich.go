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

// mediaReportEvery spaces the pass's progress reports: a probe over a network
// share takes about half a second, so a few hundred files would otherwise run
// for minutes behind a progress line that never moves.
const mediaReportEvery = 16

// enrichMediaFacts probes the entries this scan touched whose image dimensions
// or audio duration are still unknown, and writes the measured facts back
// (ADR 0015 §2). It runs after the merge, inside the scan's admission slot,
// and reports (probed, total) as it goes — first with 0, last with total.
//
// The whole pass is best-effort and deliberately cannot fail the scan: the
// merge is already committed, so reporting failure would misreport an updated
// inventory, and a fact left NULL is simply re-probed by the next scan that
// touches the entry. A prober that is not wired (nil) skips the pass, which is
// the state every pipeline test runs in.
func (s *Pipeline) enrichMediaFacts(ctx context.Context, sessionID string, report func(probed, total int)) {
	if s.prober == nil {
		return
	}
	candidates, err := s.repo.EntriesMissingMediaFacts(sessionID)
	if err != nil || len(candidates) == 0 {
		return
	}
	report(0, len(candidates))

	var updates []MediaFactsUpdate
	var mu sync.Mutex
	probed := 0
	var next atomic.Int64
	var wg sync.WaitGroup
	for range min(mediaProbeWorkers, len(candidates)) {
		wg.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= len(candidates) || ctx.Err() != nil {
					return
				}
				u, ok := s.probeMediaFacts(ctx, candidates[i])
				// One lock serializes the report too: the callback writes the
				// scan's event stream, which is not safe for concurrent use.
				mu.Lock()
				if ok {
					updates = append(updates, u)
				}
				probed++
				if probed%mediaReportEvery == 0 || probed == len(candidates) {
					report(probed, len(candidates))
				}
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if len(updates) > 0 {
		_ = s.repo.UpdateEntryMediaFacts(updates)
	}
}

// mediaReporter turns the pass's counts into the scan's "media" progress,
// carrying the walk's totals so the stream's last counts stay true.
func mediaReporter(progress func(Progress), walked Progress) func(probed, total int) {
	return func(probed, total int) {
		if progress != nil {
			progress(Progress{
				FilesScanned: walked.FilesScanned,
				DirsScanned:  walked.DirsScanned,
				Stage:        "media",
				MediaProbed:  probed,
				MediaTotal:   total,
			})
		}
	}
}

// probeMediaFacts measures one candidate; false means unmeasurable, which
// stays NULL rather than zero.
func (s *Pipeline) probeMediaFacts(ctx context.Context, c MediaCandidate) (MediaFactsUpdate, bool) {
	u := MediaFactsUpdate{Path: c.Path}
	var err error
	switch {
	case strings.HasPrefix(c.Format, "image/"):
		u.Width, u.Height, err = s.prober.ImageDimensions(c.Path)
	case strings.HasPrefix(c.Format, "audio/"):
		u.DurationSec, err = s.prober.AudioDurationSec(ctx, c.Path)
	default:
		return u, false
	}
	return u, err == nil && (u.Width > 0 || u.DurationSec > 0)
}
