package workset

import (
	"context"
	"time"
)

// GenerationSnapshot is the initial session_snapshot SSE payload. It keeps
// the wire fields expected by SSE clients (RFC3339Nano timestamps as strings).
type GenerationSnapshot struct {
	GenerationID   string `json:"generation_id"`
	WorksetID      string `json:"workset_id"`
	OperationType  string `json:"operation_type"`
	Status         string `json:"status"`
	TotalRoots     int    `json:"total_roots"`
	CompletedRoots int    `json:"completed_roots"`
	CurrentRoot    string `json:"current_root"`
	ErrorCount     int    `json:"error_count"`
	RevisionID     string `json:"revision_id"`
	ErrorCode      string `json:"error_code"`
	ErrorMessage   string `json:"error_message"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at"`
	CreatedAt      string `json:"created_at"`
}

// GenerationProgressEvent is the progress SSE payload for a planning session.
type GenerationProgressEvent struct {
	GenerationID   string `json:"generation_id"`
	TotalRoots     int    `json:"total_roots"`
	CompletedRoots int    `json:"completed_roots"`
	CurrentRoot    string `json:"current_root"`
	ErrorCount     int    `json:"error_count"`
}

// GenerationCompletedEvent is the completed SSE payload for a planning session.
type GenerationCompletedEvent struct {
	GenerationID string `json:"generation_id"`
	RevisionID   string `json:"revision_id"`
}

// GenerationFailedEvent is the failed SSE payload for a planning session.
type GenerationFailedEvent struct {
	GenerationID string `json:"generation_id"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

// GenerationStoppedEvent is the canceled/interrupted SSE payload for a planning session.
type GenerationStoppedEvent struct {
	GenerationID string `json:"generation_id"`
}

// SessionStatusEvent is the fallback message SSE payload when a session ends
// in an unexpected status.
type SessionStatusEvent struct {
	Status string `json:"status"`
}

func snapshotOf(g *PlanGeneration) GenerationSnapshot {
	return GenerationSnapshot{
		GenerationID:   g.GenerationID,
		WorksetID:      g.WorksetID,
		OperationType:  g.OperationType,
		Status:         g.Status,
		TotalRoots:     g.TotalRoots,
		CompletedRoots: g.CompletedRoots,
		CurrentRoot:    g.CurrentRoot,
		ErrorCount:     g.ErrorCount,
		RevisionID:     g.RevisionID,
		ErrorCode:      g.ErrorCode,
		ErrorMessage:   g.ErrorMessage,
		StartedAt:      formatTime(g.StartedAt),
		FinishedAt:     formatTime(g.FinishedAt),
		CreatedAt:      formatTime(g.CreatedAt),
	}
}

// formatTime renders a zero time as the empty string, matching the HTTP
// detail payload's convention.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// Subscribe streams generation lifecycle events over the emit callback.
//
// Every connection first receives a complete `session_snapshot` event (the
// current persisted state), then only future events: `progress` (root counts +
// current root), or a terminal event (`completed` with revision_id, `failed`,
// `canceled`, `interrupted`). There is no historical replay and never a fake
// percentage. Because the snapshot is emitted first, a reconnect after a
// disconnect resumes from the current truth; the detail GET remains the
// fallback when no SSE transport is available.
func (s *serviceImpl) Subscribe(
	ctx context.Context,
	worksetID, operationType, generationID string,
	emit func(event string, data any) error,
) error {
	// Scoped access check first: a session belonging to another workset or
	// another operation is not found.
	g, err := s.loadGeneration(worksetID, operationType, generationID)
	if err != nil {
		return err
	}

	// Initial snapshot: the authoritative current state, always first.
	if err := emit("session_snapshot", snapshotOf(g)); err != nil {
		return err
	}
	// Terminal sessions have no future events; the snapshot above is the
	// whole payload. Emit an explicit terminal event so clients that keyed
	// their state machine on terminal SSE events (rather than inspecting the
	// snapshot) reach a confirmed terminal instead of reporting a transport
	// error when the stream ends.
	if terminalStatus(g.Status) {
		return emitTerminal(emit, g.Status, g)
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	last := -1
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		cur, err := s.repo.GetGeneration(generationID)
		if err != nil {
			return nil // disconnect fallback: client reconnects to detail
		}
		if cur.Status != g.Status {
			if terminalStatus(cur.Status) {
				return emitTerminal(emit, cur.Status, cur)
			}
		}
		if cur.CompletedRoots != last {
			last = cur.CompletedRoots
			if err := emit("progress", GenerationProgressEvent{
				GenerationID:   cur.GenerationID,
				TotalRoots:     cur.TotalRoots,
				CompletedRoots: cur.CompletedRoots,
				CurrentRoot:    cur.CurrentRoot,
				ErrorCount:     cur.ErrorCount,
			}); err != nil {
				return err
			}
		}
		g = cur
	}
}

func terminalStatus(status string) bool {
	switch status {
	case GenStatusCompleted, GenStatusFailed, GenStatusCanceled, GenStatusInterrupted:
		return true
	}
	return false
}

// emitTerminal sends the terminal event matching the session status.
func emitTerminal(emit func(event string, data any) error, status string, g *PlanGeneration) error {
	switch status {
	case GenStatusCompleted:
		return emit("completed", GenerationCompletedEvent{
			GenerationID: g.GenerationID,
			RevisionID:   g.RevisionID,
		})
	case GenStatusFailed:
		return emit("failed", GenerationFailedEvent{
			GenerationID: g.GenerationID,
			ErrorCode:    g.ErrorCode,
			ErrorMessage: g.ErrorMessage,
		})
	case GenStatusCanceled:
		return emit("canceled", GenerationStoppedEvent{GenerationID: g.GenerationID})
	case GenStatusInterrupted:
		return emit("interrupted", GenerationStoppedEvent{GenerationID: g.GenerationID})
	default:
		return emit("message", SessionStatusEvent{Status: status})
	}
}
