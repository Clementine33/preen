package app

import (
	"context"
	"log"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/onsei/organizer/backend/internal/workset"
)

const shutdownLimit = 5 * time.Second
const httpDrainLimit = 4 * time.Second

// activeHandlers seals admission before waiting, so a late ServeHTTP cannot
// access the repository after the last admitted request returns.
type activeHandlers struct {
	next    http.Handler
	mu      sync.Mutex
	wg      sync.WaitGroup
	stopped bool
	done    chan struct{}
	active  map[*http.Request]struct{}
}

func newActiveHandlers(next http.Handler) *activeHandlers {
	return &activeHandlers{next: next, done: make(chan struct{}), active: make(map[*http.Request]struct{})}
}

func (h *activeHandlers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"SHUTTING_DOWN","message":"backend is shutting down"}`))
		return
	}
	h.wg.Add(1)
	h.active[r] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.active, r)
		h.mu.Unlock()
		h.wg.Done()
	}()
	h.next.ServeHTTP(w, r)
}

func (h *activeHandlers) seal() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.stopped {
		h.stopped = true
		go func() { h.wg.Wait(); close(h.done) }()
	}
	return h.done
}

func (h *activeHandlers) pending() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	paths := make([]string, 0, len(h.active))
	for r := range h.active {
		paths = append(paths, r.Method+" "+r.URL.Path)
	}
	slices.Sort(paths)
	return paths
}

// shutdownBackend keeps the process guard armed through joins and database
// Close. forceExit is os.Exit in production; tests can observe the timeout.
func shutdownBackend(
	server *http.Server, handlers *activeHandlers, dispatcher workset.Dispatcher,
	loops map[string]<-chan struct{}, closeDB func() error, forceExit func(int),
) {
	deadline := time.Now().Add(shutdownLimit)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	var pendingMu sync.Mutex
	pending := make(map[string]bool)
	for name := range loops {
		pending[name] = true
	}
	pending["http"], pending["handlers"], pending["dispatcher"], pending["database"] = true, true, true, true
	forcedDone := make(chan struct{})
	forcedExit := time.AfterFunc(time.Until(deadline), func() {
		pendingMu.Lock()
		remaining := make([]string, 0, len(pending))
		for name, waiting := range pending {
			if waiting {
				remaining = append(remaining, name)
			}
		}
		pendingMu.Unlock()
		slices.Sort(remaining)
		log.Printf(
			"forced shutdown timeout: pending=%v dispatcher=%v handlers=%v",
			remaining,
			dispatcher,
			handlers.pending(),
		)
		defer close(forcedDone)
		forceExit(1)
	})
	markDone := func(name string) { pendingMu.Lock(); pending[name] = false; pendingMu.Unlock() }
	handlerDone := handlers.seal()
	httpCtx, cancelHTTP := context.WithDeadline(context.Background(), deadline.Add(httpDrainLimit-shutdownLimit))
	defer cancelHTTP()
	var joined sync.WaitGroup
	joined.Go(func() { DrainHTTPServer(httpCtx, server.Shutdown, server.Close); markDone("http") })
	joined.Go(func() { <-handlerDone; markDone("handlers") })
	joined.Go(func() {
		if err := dispatcher.Stop(ctx); err != nil {
			log.Printf("shutdown: %v", err)
			return
		}
		markDone("dispatcher")
	})
	for name, done := range loops {
		joined.Go(func() { <-done; markDone(name) })
	}
	allDone := make(chan struct{})
	go func() { joined.Wait(); close(allDone) }()
	select {
	case <-ctx.Done():
		// Keep the guard armed: live handlers or workers still own the database.
		<-forcedDone
		return
	case <-allDone:
	}
	// Stop may return its deadline error just as the last join returns.
	if ctx.Err() != nil {
		<-forcedDone
		return
	}
	if err := closeDB(); err != nil {
		log.Printf("close repository: %v", err)
	}
	markDone("database")
	forcedExit.Stop()
}
