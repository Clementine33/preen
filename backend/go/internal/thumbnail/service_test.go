package thumbnail_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/onsei/organizer/backend/internal/thumbnail"
)

// gatedEngine renders only once its gate opens, counting renders: how many
// computations ran is the contract singleflight and the cache keep.
type gatedEngine struct {
	gate    chan struct{}
	renders atomic.Int32
	size    int
}

func (e *gatedEngine) Thumbnail(path string, _ int) (thumbnail.Image, error) {
	if e.gate != nil {
		<-e.gate
	}
	e.renders.Add(1)
	return thumbnail.Image{Bytes: make([]byte, e.size), ContentType: "image/jpeg"}, nil
}

func TestConcurrentRequestsShareOneRenderAndTheCacheServesTheNext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		engine := &gatedEngine{gate: make(chan struct{}), size: 10}
		svc := thumbnail.New(engine, t.TempDir(), 1<<20, 1, 4)
		src := thumbnail.Source{Path: "/lib/a.jpg", Version: "v1"}

		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				if _, err := svc.Get(context.Background(), src, 120); err != nil {
					t.Error(err)
				}
			})
		}
		synctest.Wait()
		close(engine.gate)
		wg.Wait()

		if _, err := svc.Get(context.Background(), src, 120); err != nil {
			t.Fatal(err)
		}
		if n := engine.renders.Load(); n != 1 {
			t.Errorf("renders = %d, want 1", n)
		}
	})
}

func TestAFullQueueRefusesAndALeavingRequestFreesItsPlace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		engine := &gatedEngine{gate: make(chan struct{})}
		svc := thumbnail.New(engine, t.TempDir(), 1<<20, 1, 1)
		get := func(ctx context.Context, path string) error {
			_, err := svc.Get(ctx, thumbnail.Source{Path: path, Version: "v"}, 120)
			return err
		}

		// One rendering, one queued: the third is refused.
		go func() { _ = get(context.Background(), "/a") }()
		synctest.Wait()
		ctx, cancel := context.WithCancel(context.Background())
		queued := make(chan error, 1)
		go func() { queued <- get(ctx, "/b") }()
		synctest.Wait()
		if err := get(context.Background(), "/c"); !errors.Is(err, thumbnail.ErrBusy) {
			t.Fatalf("third request: got %v, want ErrBusy", err)
		}

		// The queued request leaves; its place is free again.
		cancel()
		if err := <-queued; !errors.Is(err, context.Canceled) {
			t.Errorf("leaving request: got %v", err)
		}
		done := make(chan error, 1)
		go func() { done <- get(context.Background(), "/d") }()
		synctest.Wait()
		close(engine.gate)
		if err := <-done; err != nil {
			t.Errorf("request after the queue freed: %v", err)
		}
	})
}

func TestTheCacheEvictsPastItsBudget(t *testing.T) {
	dir := t.TempDir()
	svc := thumbnail.New(&gatedEngine{size: 400}, dir, 1000, 1, 1)
	for _, path := range []string{"/a", "/b", "/c", "/d"} {
		if _, err := svc.Get(context.Background(), thumbnail.Source{Path: path, Version: "v"}, 120); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, entry := range entries {
		info, _ := entry.Info()
		total += info.Size()
	}
	if total > 1000 {
		t.Errorf("cache holds %d bytes over a 1000 budget", total)
	}
}

func TestSnapHeight(t *testing.T) {
	for requested, want := range map[int]int{1: 120, 120: 120, 121: 240, 700: 960, 5000: 960} {
		if got, ok := thumbnail.SnapHeight(requested); !ok || got != want {
			t.Errorf("SnapHeight(%d) = %d, %v; want %d", requested, got, ok, want)
		}
	}
	if _, ok := thumbnail.SnapHeight(0); ok {
		t.Error("a zero height must be refused")
	}
}
