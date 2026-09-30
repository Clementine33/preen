package workset

import (
	"context"
	"time"
)

// executionProgressEvent is the wire shape of the progress event. It never
// carries a fabricated percentage: only counts that actually happened on disk.
type executionProgressEvent struct {
	ExecutionID         string `json:"execution_id"`
	Status              string `json:"status"`
	TotalComponents     int    `json:"total_components"`
	CompletedComponents int    `json:"completed_components"`
	TotalOperations     int    `json:"total_operations"`
	CompletedOperations int    `json:"completed_operations"`
	CurrentRoot         string `json:"current_root"`
	CurrentComponentID  string `json:"current_component_id"`
	CurrentPhase        string `json:"current_phase"`
}

// SubscribeExecution streams execution lifecycle events over the emit callback.
//
// Every connection first receives a complete `execution_snapshot` event (the
// current persisted state, per-component results included), then only future
// events: `component` for each component whose result lands, `progress` when
// the position or the counters move, and one terminal event. There is no
// historical replay; because the snapshot is emitted first, a reconnect resumes
// from the current truth, and a client that fell behind is brought up to date
// by the detail GET (or by the terminal event's calibration) rather than by a
// stream of past events.
func (s *serviceImpl) SubscribeExecution(
	ctx context.Context,
	worksetID, operationType, executionID string,
	emit func(event string, data any) error,
) error {
	e, err := s.loadExecution(worksetID, operationType, executionID)
	if err != nil {
		return err
	}
	snapshot, err := s.executionViewOf(e, ExecutionPage{})
	if err != nil {
		return err
	}
	if emitErr := emit(ExecutionEventSnapshot, snapshot); emitErr != nil {
		return emitErr
	}
	if executionTerminal(e.Status) {
		return emitExecutionTerminal(emit, e)
	}

	// The frozen worklist never changes. Audio commits advance a cursor; batch
	// results can arrive in any order, so those streams track delivered indices.
	req, err := parseExecutionRequest(e.RequestJSON)
	if err != nil {
		return NewError(ErrKindInternal, "REQUEST_LOAD_FAILED", "failed to load the frozen request", err)
	}
	identity := initialComponentReport(req.Units)
	at := make(map[int]int, len(identity))
	for i := range identity {
		at[identity[i].ComponentIndex] = i
	}
	_, unordered := s.batchExecutors[operationType]
	seen := make(map[int]bool, len(snapshot.Components))
	cursor := 0
	// Seed from what the client actually saw, not a later database read.
	for _, entry := range snapshot.Components {
		if entry.Status == ExecComponentPending {
			continue
		}
		seen[entry.ComponentIndex] = true
		if !unordered {
			cursor = max(cursor, entry.ComponentIndex+1)
		}
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	last := progressOf(e)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		cur, loadErr := s.repo.GetExecutionProgress(executionID)
		if loadErr != nil {
			return nil // disconnect fallback: client reconnects to the detail route
		}
		// The results go out before the terminal event even when the run ended
		// between two ticks: a client that only ever saw the terminal event
		// would have to re-read the detail to learn what the last components did.
		results, listErr := s.repo.ListExecutionComponentResults(executionID, cursor, 0)
		if listErr != nil {
			return nil // disconnect fallback, as above
		}
		for _, res := range results {
			if unordered && seen[res.ComponentIndex] {
				continue
			}
			idx, ok := at[res.ComponentIndex]
			if !ok {
				continue // a result that belongs to no frozen component
			}
			entry := identity[idx]
			applyComponentResult(&entry, res)
			if emitErr := emit(ExecutionEventComponent, entry); emitErr != nil {
				return emitErr
			}
			seen[res.ComponentIndex] = true
			if !unordered {
				cursor = res.ComponentIndex + 1
			}
		}
		if executionTerminal(cur.Status) {
			return emitExecutionTerminal(emit, cur)
		}
		if next := progressOf(cur); next != last {
			last = next
			if emitErr := emit(ExecutionEventProgress, next); emitErr != nil {
				return emitErr
			}
		}
	}
}

// progressOf builds the progress payload a reconnect or tick compares against.
func progressOf(e *PlanExecution) executionProgressEvent {
	return executionProgressEvent{
		ExecutionID:         e.ExecutionID,
		Status:              e.Status,
		TotalComponents:     e.TotalComponents,
		CompletedComponents: e.CompletedComponents,
		TotalOperations:     e.TotalOperations,
		CompletedOperations: e.CompletedOperations,
		CurrentRoot:         e.CurrentRoot,
		CurrentComponentID:  e.CurrentComponentID,
		CurrentPhase:        e.CurrentPhase,
	}
}

// executionTerminal reports whether a status is final.
func executionTerminal(status string) bool {
	switch status {
	case ExecStatusSucceeded, ExecStatusFailed,
		ExecStatusCanceled, ExecStatusInterrupted:
		return true
	}
	return false
}

// emitExecutionTerminal sends the terminal event matching the session status.
func emitExecutionTerminal(emit func(event string, data any) error, e *PlanExecution) error {
	switch e.Status {
	case ExecStatusSucceeded:
		return emit(ExecutionEventSucceeded, map[string]any{
			"execution_id": e.ExecutionID,
			"plan_id":      e.PlanID,
		})
	case ExecStatusFailed:
		return emit(ExecutionEventFailed, map[string]any{
			"execution_id":  e.ExecutionID,
			"error_code":    e.ErrorCode,
			"error_message": e.ErrorMessage,
		})
	case ExecStatusCanceled:
		return emit(ExecutionEventCanceled, map[string]any{"execution_id": e.ExecutionID})
	case ExecStatusInterrupted:
		return emit(ExecutionEventInterrupted, map[string]any{"execution_id": e.ExecutionID})
	default:
		return emit("message", map[string]any{"status": e.Status})
	}
}
