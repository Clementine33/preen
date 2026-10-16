package inventory_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/onsei/organizer/backend/internal/admission"
	"github.com/onsei/organizer/backend/internal/inventory"
)

// factsStore is an in-memory inventory of files with unknown media facts.
type factsStore struct {
	mu       sync.Mutex
	missing  map[string]string // path -> format
	written  map[string]inventory.MediaFactsUpdate
	writeErr error
}

func newFactsStore(n int) *factsStore {
	s := &factsStore{missing: map[string]string{}, written: map[string]inventory.MediaFactsUpdate{}}
	for i := range n {
		format := "audio/mpeg"
		if i%5 == 0 {
			format = "image/png"
		}
		s.missing[fmt.Sprintf("/lib/rj1/%04d", i)] = format
	}
	return s
}

func (s *factsStore) EntriesMissingMediaFacts(after string, limit int) ([]inventory.MediaCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []inventory.MediaCandidate
	for path, format := range s.missing {
		if path > after {
			out = append(out, inventory.MediaCandidate{Path: path, Format: format})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out[:min(limit, len(out))], nil
}

func (s *factsStore) UpdateEntryMediaFacts(updates []inventory.MediaFactsUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writeErr != nil {
		return s.writeErr
	}
	for _, u := range updates {
		delete(s.missing, u.Path)
		s.written[u.Path] = u
	}
	return nil
}

// MediaFactsCounts files every path under one library.
func (s *factsStore) MediaFactsCounts(after string) ([]inventory.LibraryMediaCounts, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := inventory.LibraryMediaCounts{LibraryID: "lib"}
	for path := range s.missing {
		if path > after {
			c.Pending++
		} else {
			c.Unmeasurable++
		}
	}
	if c.Pending+c.Unmeasurable == 0 {
		return nil, nil
	}
	return []inventory.LibraryMediaCounts{c}, nil
}

func TestMediaFactsNotifyOnlyCommittedBatchesAndCoalesce(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("write failure %v", fail), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				store := newFactsStore(130)
				if fail {
					store.writeErr = errors.New("write failed")
				}
				e := inventory.NewMediaEnricher(store, &slowProber{}, nil)
				changes, unsubscribe := e.Subscribe()
				<-changes // initial reload
				runEnricher(t, e)
				e.Kick()
				time.Sleep(time.Second)
				synctest.Wait()
				want := 1
				if fail {
					want = 0
				}
				if len(changes) != want {
					t.Errorf("pending signals = %d, want %d", len(changes), want)
				}
				if !fail {
					<-changes
					if missing, written := store.counts(); missing != 0 || written != 130 {
						t.Errorf("notification preceded commit: missing %d written %d", missing, written)
					}
				}
				unsubscribe()
				unsubscribe() // cancellation is idempotent
				store.mu.Lock()
				store.missing["/lib/another.wav"] = "audio/wav"
				store.mu.Unlock()
				e.Kick()
				time.Sleep(time.Second)
				if len(changes) != 0 {
					t.Error("unsubscribed reader received a notification")
				}
				fresh, stop := e.Subscribe()
				defer stop()
				if len(fresh) != 1 {
					t.Error("reconnecting reader missed the initial reload")
				}
			})
		})
	}
}

func (s *factsStore) counts() (missing, written int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.missing), len(s.written)
}

// slowProber takes 10ms a probe, stops when its context ends, and cannot
// measure the paths in fail. It counts what it was asked.
type slowProber struct {
	mu    sync.Mutex
	fail  map[string]bool
	asked int
}

func (p *slowProber) answer(ctx context.Context, path string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Millisecond):
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked++
	if p.fail[path] {
		return errors.New("unmeasurable")
	}
	return nil
}

func (p *slowProber) ImageDimensions(path string) (int, int, error) {
	if err := p.answer(context.Background(), path); err != nil {
		return 0, 0, err
	}
	return 1200, 900, nil
}

func (p *slowProber) AudioFacts(ctx context.Context, path string) (float64, int64, error) {
	if err := p.answer(ctx, path); err != nil {
		return 0, 0, err
	}
	return 185.5, 320000, nil
}

func (p *slowProber) askedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.asked
}

// runEnricher serves kicks until the test's bubble ends.
func runEnricher(t *testing.T, e *inventory.MediaEnricher) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func TestMediaEnricherMeasuresEveryCandidateOncePerKick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newFactsStore(150) // more than two batches
		prober := &slowProber{fail: map[string]bool{"/lib/rj1/0007": true, "/lib/rj1/0100": true}}
		e := inventory.NewMediaEnricher(store, prober, nil)
		runEnricher(t, e)

		e.Kick()
		time.Sleep(time.Minute)
		missing, written := store.counts()
		if missing != 2 || written != 148 {
			t.Fatalf("missing %d, written %d; want the two unmeasurable left, the rest written", missing, written)
		}
		if u := store.written["/lib/rj1/0005"]; u.Width != 1200 || u.Height != 900 {
			t.Errorf("image fact = %+v", u)
		}
		if u := store.written["/lib/rj1/0001"]; u.DurationSec != 185.5 || u.Bitrate != 320000 {
			t.Errorf("audio facts = %+v", u)
		}
		// The unmeasurable stay NULL and are not retried until the next kick:
		// the count is the contract, a pass must not loop on broken files.
		if asked := prober.askedCount(); asked != 150 {
			t.Fatalf("probes after one kick = %d, want 150", asked)
		}
		e.Kick()
		time.Sleep(time.Minute)
		if asked := prober.askedCount(); asked != 152 {
			t.Errorf("probes after a second kick = %d, want 152 (only the two unknowns again)", asked)
		}
	})
}

func TestMediaEnricherYieldsToAScanAndResumes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := admission.NewGate(nil)
		store := newFactsStore(200)
		e := inventory.NewMediaEnricher(store, &slowProber{}, gate.BeginBackground)
		e.Workers = 4
		runEnricher(t, e)

		e.Kick()
		time.Sleep(25 * time.Millisecond) // mid-way through the first batch

		releaseScan, err := gate.BeginScan()
		if err != nil {
			t.Fatalf("a scan must preempt the pass, not be refused: %v", err)
		}
		// What was measured before the stop is already written: the scan only
		// starts after the pass's last write.
		_, atStop := store.counts()
		if atStop == 0 || atStop == 200 {
			t.Fatalf("written at the scan's start = %d, want the measured part of the first batch", atStop)
		}
		time.Sleep(10 * time.Second) // retries are refused while the scan runs
		if _, written := store.counts(); written != atStop {
			t.Fatalf("the pass wrote %d facts during the scan", written-atStop)
		}

		releaseScan()
		time.Sleep(time.Minute)
		if missing, _ := store.counts(); missing != 0 {
			t.Errorf("after the scan the pass must resume and finish; %d left", missing)
		}
	})
}

func TestMediaFactsStatusFollowsPreemptionAndResume(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := admission.NewGate(nil)
		e := inventory.NewMediaEnricher(newFactsStore(200), &slowProber{}, gate.BeginBackground)
		e.Workers = 4
		runEnricher(t, e)
		signals, release := e.SubscribeStatus()
		defer release()
		<-signals // connect-time signal
		if st := e.Status(); st.State != inventory.MediaFactsIdle || len(st.Libraries) != 0 {
			t.Fatalf("status before any pass = %+v", st)
		}

		e.Kick()
		time.Sleep(25 * time.Millisecond)
		if st := e.Status(); st.State != inventory.MediaFactsRunning {
			t.Fatalf("status during the pass = %+v, want running", st)
		}
		releaseScan, err := gate.BeginScan()
		if err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if st := e.Status(); st.State != inventory.MediaFactsPaused {
			t.Fatalf("status after preemption = %+v, want paused", st)
		}
		time.Sleep(10 * time.Second) // retries are refused while the scan runs
		if st := e.Status(); st.State != inventory.MediaFactsPaused {
			t.Fatalf("status while refused = %+v, want paused", st)
		}

		releaseScan()
		for e.Status().State == inventory.MediaFactsPaused {
			time.Sleep(100 * time.Millisecond)
		}
		if st := e.Status(); st.State != inventory.MediaFactsRunning {
			t.Fatalf("status after resume = %+v, want running", st)
		}
		time.Sleep(time.Minute)
		if st := e.Status(); st.State != inventory.MediaFactsIdle || len(st.Libraries) != 0 {
			t.Errorf("status after the pass = %+v, want idle with nothing left", st)
		}
	})
}

func TestMediaFactsStatusSplitsAtTheCursorAndThrottlesBatches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newFactsStore(400) // seven batches, a bit over 300ms each
		prober := &slowProber{
			fail: map[string]bool{"/lib/rj1/0007": true, "/lib/rj1/0100": true, "/lib/rj1/0300": true},
		}
		e := inventory.NewMediaEnricher(store, prober, nil)
		e.Workers = 2
		runEnricher(t, e)

		e.Kick()
		time.Sleep(400 * time.Millisecond) // first batch committed, under a second since the count
		if st := e.Status(); st.State != inventory.MediaFactsRunning || st.Libraries[0].Pending != 400 {
			t.Errorf("status inside the throttle window = %+v, want the start counts", st)
		}
		time.Sleep(time.Second) // the fourth batch (cursor 0255) ends past the window
		want := inventory.LibraryMediaCounts{LibraryID: "lib", Pending: 144, Unmeasurable: 2}
		if st := e.Status(); st.State != inventory.MediaFactsRunning || len(st.Libraries) != 1 ||
			st.Libraries[0] != want {
			t.Errorf("status after a committed batch = %+v, want %+v", st, want)
		}
		time.Sleep(time.Minute)
		want = inventory.LibraryMediaCounts{LibraryID: "lib", Unmeasurable: 3}
		if st := e.Status(); st.State != inventory.MediaFactsIdle || len(st.Libraries) != 1 || st.Libraries[0] != want {
			t.Errorf("status when idle = %+v, want every remaining file unmeasurable: %+v", st, want)
		}
	})
}
