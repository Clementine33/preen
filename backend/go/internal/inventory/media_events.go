package inventory

// Subscribe reports committed media facts (ADR 0013). The initial signal
// covers changes made before subscribing or during a disconnected interval.
// Each subscriber keeps one pending signal: readers reload the inventory,
// so a slow reader needs no history and never holds up the writer.
func (e *MediaEnricher) Subscribe() (<-chan struct{}, func()) {
	return e.hub.Subscribe(true)
}

func (e *MediaEnricher) notify() { e.hub.Notify() }
