// Package notify holds the coalescing change signal shared by the root
// notification stream's sources: a subscriber keeps one pending signal, so a
// burst of changes costs one reload and a slow reader never holds up a writer.
package notify

import "sync"

// Hub fans one "something changed" signal out to its subscribers.
type Hub struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

// Subscribe returns the subscriber's signal channel and its release. primed
// starts the channel with one pending signal, covering changes made before
// subscribing or during a disconnected interval.
func (h *Hub) Subscribe(primed bool) (<-chan struct{}, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		h.subs = make(map[chan struct{}]struct{})
	}
	ch := make(chan struct{}, 1)
	h.subs[ch] = struct{}{}
	if primed {
		ch <- struct{}{}
	}
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.subs, ch)
	}
}

// Notify marks every subscriber as having a change pending.
func (h *Hub) Notify() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
