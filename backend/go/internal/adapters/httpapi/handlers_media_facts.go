package httpapi

import (
	"io"
	"net/http"
	"time"
)

// mediaFactsEvents tells readers to reload committed inventory facts (ADR 0013).
// It carries no file paths, history, or admission slot.
func (s *Server) mediaFactsEvents(w http.ResponseWriter, r *http.Request) {
	if s.deps.MediaFacts == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "media facts are not configured")
		return
	}
	sw, err := newSSEWriter(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "streaming not supported")
		return
	}
	changes, unsubscribe := s.deps.MediaFacts.Subscribe()
	defer unsubscribe()
	// A nil channel never fires, so a server without the works hub keeps the
	// stream as it was. The subscription is primed: its first signal is the
	// connect-time event, registered before any later commit, so a commit between
	// a reader's list and this connection is never lost.
	var worksChanged <-chan struct{}
	if s.deps.WorksChanged != nil {
		var release func()
		worksChanged, release = s.deps.WorksChanged.Subscribe(true)
		defer release()
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-changes:
			if err := sw.Send("changed", struct{}{}); err != nil {
				return
			}
		case <-worksChanged:
			if err := sw.Send("works_changed", struct{}{}); err != nil {
				return
			}
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
				return
			}
			sw.flusher.Flush()
		}
	}
}
