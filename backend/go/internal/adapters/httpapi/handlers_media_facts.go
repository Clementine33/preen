package httpapi

import (
	"io"
	"net/http"
	"time"
)

// mediaFactsEvents tells readers to reload committed inventory facts (ADR
// 0015 §2). It carries no file paths, history, or admission slot.
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
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
				return
			}
			sw.flusher.Flush()
		}
	}
}
