package inventory_test

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/Clementine33/preen/backend/internal/inventory"
)

type storageFactsStore struct {
	*factsStore

	allSSD bool
}

func (s *storageFactsStore) EntriesMissingMediaFacts(after string, limit int) ([]inventory.MediaCandidate, error) {
	batch, err := s.factsStore.EntriesMissingMediaFacts(after, limit)
	for i := range batch {
		batch[i].ProbeWorkers = 16
		if !s.allSSD && i == 0 {
			batch[i].ProbeWorkers = 8
		}
	}
	return batch, err
}

type batchProber struct {
	mu      sync.Mutex
	gates   [2]chan struct{}
	running [2]int
	peak    [2]int
}

func (p *batchProber) AudioFacts(ctx context.Context, path string) (float64, int64, error) {
	n, _ := strconv.Atoi(path[strings.LastIndex(path, "/")+1:])
	batch := min(n/64, 1)
	p.mu.Lock()
	p.running[batch]++
	p.peak[batch] = max(p.peak[batch], p.running[batch])
	p.mu.Unlock()
	select {
	case <-p.gates[batch]:
	case <-ctx.Done():
	}
	p.mu.Lock()
	p.running[batch]--
	p.mu.Unlock()
	return 1, 128000, ctx.Err()
}
func (p *batchProber) ImageDimensions(ctx context.Context, path string) (int, int, error) {
	_, _, err := p.AudioFacts(ctx, path)
	return 10, 10, err
}

func TestMediaBatchUsesMostConservativeLibraryAndFreezesItsWidth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &storageFactsStore{factsStore: newFactsStore(128)}
		prober := &batchProber{gates: [2]chan struct{}{make(chan struct{}), make(chan struct{})}}
		e := inventory.NewMediaEnricher(store, prober, nil)
		runEnricher(t, e)
		e.Kick()
		synctest.Wait()
		if prober.running[0] != 8 {
			t.Fatalf("mixed-library batch running=%d, want 8", prober.running[0])
		}
		store.allSSD = true
		synctest.Wait()
		if prober.running[0] != 8 {
			t.Fatalf("in-flight batch changed width to %d", prober.running[0])
		}
		close(prober.gates[0])
		synctest.Wait()
		if prober.running[1] != 16 {
			t.Fatalf("subsequent SSD batch running=%d, want 16", prober.running[1])
		}
		close(prober.gates[1])
		synctest.Wait()
		if prober.peak != [2]int{8, 16} {
			t.Fatalf("batch peaks=%v", prober.peak)
		}
		if missing, written := store.counts(); missing != 0 || written != 128 {
			t.Fatalf("missing=%d written=%d", missing, written)
		}
	})
}
