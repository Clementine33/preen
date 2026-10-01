package inventory

// Subscribe reports committed media facts (ADR 0015 §2). The initial signal
// covers changes made before subscribing or during a disconnected interval.
// Each subscriber keeps one pending signal: readers reload the inventory,
// so a slow reader needs no history and never holds up the writer.
func (e *MediaEnricher) Subscribe() (<-chan struct{}, func()) {
	e.subMu.Lock()
	defer e.subMu.Unlock()
	if e.subs == nil {
		e.subs = make(map[chan struct{}]struct{})
	}
	ch := make(chan struct{}, 1)
	e.subs[ch] = struct{}{}
	ch <- struct{}{}
	return ch, func() {
		e.subMu.Lock()
		defer e.subMu.Unlock()
		delete(e.subs, ch)
	}
}

func (e *MediaEnricher) notify() {
	e.subMu.Lock()
	defer e.subMu.Unlock()
	for ch := range e.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
