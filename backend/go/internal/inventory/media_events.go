package inventory

import "time"

// Subscribe reports committed media facts (ADR 0013). The initial signal
// covers changes made before subscribing or during a disconnected interval.
// Each subscriber keeps one pending signal: readers reload the inventory,
// so a slow reader needs no history and never holds up the writer.
func (e *MediaEnricher) Subscribe() (<-chan struct{}, func()) {
	return e.hub.Subscribe(true)
}

func (e *MediaEnricher) notify() { e.hub.Notify() }

// MediaFactsState is what the Media Facts Pass is doing right now.
type MediaFactsState string

const (
	// MediaFactsRunning: a pass holds the admission slot and is measuring.
	MediaFactsRunning MediaFactsState = "running"
	// MediaFactsPaused: a kicked pass is refused or preempted and waits to retry.
	MediaFactsPaused MediaFactsState = "paused"
	// MediaFactsIdle: no pass is in progress.
	MediaFactsIdle MediaFactsState = "idle"
)

// LibraryMediaCounts splits one library's files that still miss media facts at
// the pass cursor: Pending files lie after it and will still be visited,
// Unmeasurable ones were visited this pass and stay unknown.
type LibraryMediaCounts struct {
	LibraryID    string
	Pending      int
	Unmeasurable int
}

// MediaFactsStatus is the pass's observable state; libraries with nothing
// missing are omitted.
type MediaFactsStatus struct {
	State     MediaFactsState
	Libraries []LibraryMediaCounts
}

// statusCountInterval throttles the counts that follow committed batches;
// state changes always recount.
const statusCountInterval = time.Second

// pastEveryPath is a cursor after which no path lies: once the pass ends every
// remaining candidate has been visited.
const pastEveryPath = "\U0010FFFF"

// SubscribeStatus signals changes of Status. Like Subscribe it coalesces to one
// pending signal and is primed, so a reader fetches the current Status on connect.
func (e *MediaEnricher) SubscribeStatus() (<-chan struct{}, func()) {
	return e.statusHub.Subscribe(true)
}

// Status returns the latest published status without touching the store.
func (e *MediaEnricher) Status() MediaFactsStatus {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()
	return e.status
}

// publish records a status for the pass at the given cursor. A state change
// always recounts; a repeat of the same state is published only after a
// committed batch (batch) and at most once per statusCountInterval. A failed
// count keeps the previous counts: the state still moves.
func (e *MediaEnricher) publish(state MediaFactsState, after string, batch bool) {
	e.statusMu.Lock()
	changed := e.status.State != state
	due := time.Since(e.countedAt) >= statusCountInterval
	libraries := e.status.Libraries
	e.statusMu.Unlock()
	if !changed && (!batch || !due) {
		return
	}
	if counts, err := e.store.MediaFactsCounts(after); err == nil {
		libraries = counts
	}
	e.statusMu.Lock()
	e.status = MediaFactsStatus{State: state, Libraries: libraries}
	e.countedAt = time.Now()
	e.statusMu.Unlock()
	e.statusHub.Notify()
}
