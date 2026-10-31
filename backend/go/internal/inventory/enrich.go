package inventory

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Clementine33/preen/backend/internal/notify"
)

const (
	// mediaProbeWorkers is the fallback for candidates without a library limit.
	mediaProbeWorkers = 8
	// mediaBatchSize bounds how much a preempted pass can lose: facts are
	// written back after every batch, and once more when the pass is stopped.
	mediaBatchSize = 64
	// mediaRetryDelay paces the retries of a pass that was refused admission or
	// preempted; a kicked pass keeps retrying until it has run to the end.
	mediaRetryDelay = 2 * time.Second
)

// MediaFactsStore is the inventory the enricher reads its candidates from and
// writes measured facts back to.
type MediaFactsStore interface {
	// EntriesMissingMediaFacts lists, in (directory path, filename) order after the given path, up to
	// limit files whose dimensions, duration or encoded bitrate are still
	// unknown.
	EntriesMissingMediaFacts(after string, limit int) ([]MediaCandidate, error)
	UpdateEntryMediaFacts(updates []MediaFactsUpdate) error
	// MediaFactsCounts counts, per library, the files still missing facts,
	// split in the same (directory path, filename) order as candidates.
	MediaFactsCounts(after string) ([]LibraryMediaCounts, error)
}

// BeginBackground takes the preemptible admission slot: stop is called when
// other work needs the slot, and the returned release frees it.
type BeginBackground func(stop func()) (release func(), err error)

// MediaEnricher measures the media facts the inventory does not know yet -
// image dimensions, audio duration, the bitrate of encoded audio - in the
// background, after scans or exact synchronization record entries (ADR 0013).
// It never runs alongside a scan's walk or anything else the admission slot serializes: it holds the
// preemptible slot, and whatever arrives stops it and waits for its last write.
//
// A pass runs only when kicked (a scan or member refresh merged, exact file
// synchronization changed files, or the process started), so a file that cannot
// be measured is retried once per kick, never in a loop. Unknown is a value, not
// a failure: the facts stay NULL.
type MediaEnricher struct {
	store  MediaFactsStore
	prober MediaProber
	begin  BeginBackground
	kick   chan struct{}
	hub    notify.Hub

	// status is the pass's published state (see publish); statusHub signals its
	// changes, separately from hub so measured facts and progress stay distinct.
	statusHub notify.Hub
	statusMu  sync.Mutex
	status    MediaFactsStatus
	countedAt time.Time

	// Workers overrides the probe width; zero uses the candidate library limits.
	Workers int
}

// NewMediaEnricher creates the enricher. A nil begin runs every pass without
// admission, the state tests run in.
func NewMediaEnricher(store MediaFactsStore, prober MediaProber, begin BeginBackground) *MediaEnricher {
	return &MediaEnricher{
		store: store, prober: prober, begin: begin, kick: make(chan struct{}, 1),
		status: MediaFactsStatus{State: MediaFactsIdle, Libraries: []LibraryMediaCounts{}},
	}
}

// Kick asks for a pass. It never blocks: a kick that arrives while a pass is
// running is kept, and makes that pass start over from the beginning at its
// next admission attempt (see pass).
func (e *MediaEnricher) Kick() {
	select {
	case e.kick <- struct{}{}:
	default:
	}
}

// Run serves kicks until ctx is cancelled.
func (e *MediaEnricher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.kick:
		}
		e.pass(ctx)
	}
}

// pass walks every candidate once, in (directory path, filename) order, resuming after each refusal
// or preemption from the last batch it finished. A kick that arrived meanwhile
// sends it back to the beginning before its next admission attempt, and one
// pending at the end keeps it running instead of publishing idle, so files
// before the cursor are always ones measured since the latest kick.
func (e *MediaEnricher) pass(ctx context.Context) {
	after := ""
	for {
		select {
		case <-e.kick:
			after = ""
		default:
		}
		if e.admitted(ctx, &after) {
			if len(e.kick) > 0 {
				continue
			}
			e.settle(ctx, MediaFactsIdle, pastEveryPath)
			return
		}
		e.settle(ctx, MediaFactsPaused, after)
		if !sleep(ctx, mediaRetryDelay) {
			return
		}
	}
}

// settle publishes a state change unless the process is shutting down, when the
// store may already be closing.
func (e *MediaEnricher) settle(ctx context.Context, state MediaFactsState, after string) {
	if ctx.Err() == nil {
		e.publish(state, after, false)
	}
}

// admitted runs batches while it holds the slot. It reports true when the pass
// is over: every candidate visited, or the store failing (the next kick tries
// again). False means refused or preempted: retry later.
func (e *MediaEnricher) admitted(ctx context.Context, after *string) bool {
	passCtx, stop := context.WithCancel(ctx)
	defer stop()
	release := func() {}
	if e.begin != nil {
		var err error
		if release, err = e.begin(stop); err != nil {
			return false
		}
	}
	defer release()
	e.settle(ctx, MediaFactsRunning, *after)

	for passCtx.Err() == nil {
		batch, err := e.store.EntriesMissingMediaFacts(*after, mediaBatchSize)
		if err != nil || len(batch) == 0 {
			return true
		}
		updates := e.measure(passCtx, batch)
		// Written even when stopped mid-batch: the facts are measured, and the
		// release (deferred above) waits for this write.
		if len(updates) > 0 {
			if err := e.store.UpdateEntryMediaFacts(updates); err != nil {
				return true
			}
			e.notify()
		}
		if passCtx.Err() != nil {
			break
		}
		// Only a whole batch moves the cursor; a stopped one is asked again,
		// and its measured files no longer qualify.
		*after = batch[len(batch)-1].Path
		e.publish(MediaFactsRunning, *after, true)
	}
	return ctx.Err() != nil
}

// measure freezes the most conservative candidate limit for the whole batch.
func (e *MediaEnricher) measure(ctx context.Context, batch []MediaCandidate) []MediaFactsUpdate {
	workers := e.Workers
	if workers <= 0 {
		workers = len(batch)
		for _, candidate := range batch {
			width := candidate.ProbeWorkers
			if width <= 0 {
				width = mediaProbeWorkers
			}
			workers = min(workers, width)
		}
	}
	var updates []MediaFactsUpdate
	var mu sync.Mutex
	var next atomic.Int64
	var wg sync.WaitGroup
	for range min(workers, len(batch)) {
		wg.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= len(batch) || ctx.Err() != nil {
					return
				}
				if u, ok := e.probe(ctx, batch[i]); ok {
					mu.Lock()
					updates = append(updates, u)
					mu.Unlock()
				}
			}
		})
	}
	wg.Wait()
	return updates
}

// probe measures one candidate; false means unmeasurable, which stays NULL
// rather than zero.
func (e *MediaEnricher) probe(ctx context.Context, c MediaCandidate) (MediaFactsUpdate, bool) {
	u := MediaFactsUpdate{Path: c.Path}
	var err error
	switch {
	case strings.HasPrefix(c.Format, "image/"):
		u.Width, u.Height, err = e.prober.ImageDimensions(ctx, c.Path)
	case strings.HasPrefix(c.Format, "audio/"):
		u.DurationSec, u.Bitrate, err = e.prober.AudioFacts(ctx, c.Path)
	default:
		return u, false
	}
	return u, err == nil && (u.Width > 0 || u.DurationSec > 0 || u.Bitrate > 0)
}

// sleep waits d and reports false when ctx ends first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
