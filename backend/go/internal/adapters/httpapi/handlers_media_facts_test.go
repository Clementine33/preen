package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/onsei/organizer/backend/internal/adapters/httpapi"
	"github.com/onsei/organizer/backend/internal/inventory"
)

// flushedResponse snapshots on the writer goroutine; the test only reads
// immutable strings until the handler has stopped using the recorder.
type flushedResponse struct {
	*httptest.ResponseRecorder

	flushes chan string
}

func (w *flushedResponse) Flush() {
	w.ResponseRecorder.Flush()
	w.flushes <- w.Body.String()
}

func TestMediaFactsStreamConnectsHeartbeatsAndCancels(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := httpapi.NewServer(httpapi.Dependencies{
			Token: "test", MediaFacts: inventory.NewMediaEnricher(nil, nil, nil),
		})
		ctx, cancel := context.WithCancel(context.Background())
		r := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/media-facts/events", nil).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer test")
		w := &flushedResponse{ResponseRecorder: httptest.NewRecorder(), flushes: make(chan string, 1)}
		done := make(chan struct{})
		go func() {
			defer close(done)
			h.ServeHTTP(w, r)
		}()
		if got := <-w.flushes; got != "event: changed\ndata: {}\n\n" {
			t.Errorf("initial notification = %q", got)
		}
		started := time.Now()
		if got := <-w.flushes; !strings.Contains(got, ": keep-alive\n\n") {
			t.Error("missing heartbeat")
		}
		if elapsed := time.Since(started); elapsed != 15*time.Second {
			t.Errorf("heartbeat after %s, want 15s", elapsed)
		}
		cancel()
		<-done
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/event-stream" || !w.Flushed {
			t.Errorf("stream response = %d %v, flushed %v", w.Code, w.Header(), w.Flushed)
		}
	})
}

func TestMediaFactsStreamRequiresHeaderAuthentication(t *testing.T) {
	h := httpapi.NewServer(httpapi.Dependencies{Token: "test"})
	for _, target := range []string{"/api/v1/inventory/media-facts/events", "/api/v1/inventory/media-facts/events?access_token=test"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s = %d, want 401", target, w.Code)
		}
	}
}
