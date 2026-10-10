//nolint:testpackage // Exercise private shutdown admission and watchdog without adding production APIs.
package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type shutdownDispatcher struct{ stop func(context.Context) error }

func (shutdownDispatcher) Start()                           {}
func (d shutdownDispatcher) Stop(ctx context.Context) error { return d.stop(ctx) }

func TestShutdownJoinsHandlersAndLoopsBeforeClosing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, release := make(chan struct{}), make(chan struct{})
		var calls, closes, exits atomic.Int32
		h := newActiveHandlers(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			calls.Add(1)
			close(entered)
			<-release
		}))
		go h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/active", nil))
		<-entered
		loopDone, workerDone, shutdownDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
		d := shutdownDispatcher{stop: func(context.Context) error { <-workerDone; return nil }}
		go func() {
			shutdownBackend(&http.Server{}, h, d, map[string]<-chan struct{}{"covers": loopDone},
				func() error { closes.Add(1); return nil }, func(int) { exits.Add(1) })
			close(shutdownDone)
		}()
		synctest.Wait()
		late := httptest.NewRecorder()
		h.ServeHTTP(late, httptest.NewRequest(http.MethodPost, "/late", nil))
		if late.Code != http.StatusServiceUnavailable || calls.Load() != 1 {
			t.Fatal("late request entered business handler")
		}
		close(loopDone)
		close(workerDone)
		synctest.Wait()
		if closes.Load() != 0 {
			t.Fatal("database closed while handler was running")
		}
		close(release)
		<-shutdownDone
		time.Sleep(6 * time.Second)
		if closes.Load() != 1 || exits.Load() != 0 {
			t.Fatalf("close=%d exit=%d", closes.Load(), exits.Load())
		}
	})
}

func TestHandlerAdmissionCanRaceWithSeal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		h := newActiveHandlers(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
		var wg sync.WaitGroup
		for range 100 {
			wg.Go(func() { h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/race", nil)) })
		}
		<-h.seal()
		before := calls.Load()
		wg.Wait()
		if calls.Load() != before {
			t.Fatal("handler started after sealed accounting drained")
		}
	})
}

func TestHandlerJoinOutlivesClosedConnection(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	h := newActiveHandlers(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	}))
	server := httptest.NewServer(h)
	defer server.Close()
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		response, err := server.Client().Get(server.URL)
		if err == nil {
			response.Body.Close()
		}
	}()
	<-entered
	done := h.seal()
	if err := server.Config.Close(); err != nil {
		t.Fatal(err)
	}
	<-clientDone
	select {
	case <-done:
		t.Fatal("closing the connection incorrectly completed handler accounting")
	default:
	}
	close(release)
	<-done
}

func TestShutdownForcedExit(t *testing.T) {
	if mode := os.Getenv("DORGANIZER_SHUTDOWN_TEST"); mode != "" {
		marker := os.Getenv("DORGANIZER_SHUTDOWN_MARKER")
		d := shutdownDispatcher{stop: func(ctx context.Context) error {
			if mode == "worker" {
				<-ctx.Done()
				return ctx.Err()
			}
			return nil
		}}
		h := newActiveHandlers(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		shutdownBackend(&http.Server{}, h, d, nil, func() error {
			if err := os.WriteFile(marker, []byte("close entered"), 0o600); err != nil {
				panic(err)
			}
			select {}
		}, os.Exit)
		t.Fatal("shutdown returned without forced exit")
	}
	for _, mode := range []string{"worker", "close"} {
		t.Run(mode, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "closed")
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestShutdownForcedExit$")
			cmd.Env = append(os.Environ(), "DORGANIZER_SHUTDOWN_TEST="+mode, "DORGANIZER_SHUTDOWN_MARKER="+marker)
			start := time.Now()
			output, err := cmd.CombinedOutput()
			var exited *exec.ExitError
			if !errors.As(err, &exited) || exited.ExitCode() != 1 {
				t.Fatalf("exit=%v output=%s", err, output)
			}
			if elapsed := time.Since(start); elapsed < 5*time.Second || elapsed > 9*time.Second {
				t.Fatalf("elapsed=%s output=%s", elapsed, output)
			}
			_, statErr := os.Stat(marker)
			if mode == "worker" && !errors.Is(statErr, os.ErrNotExist) {
				t.Fatal("database close ran with a live worker")
			}
			if mode == "close" && statErr != nil {
				t.Fatalf("database close did not start: %v", statErr)
			}
		})
	}
}
