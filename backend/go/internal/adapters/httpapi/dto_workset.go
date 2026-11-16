package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// ==================== Draft & Revision DTOs ====================

type audioQualityDTO struct {
	Kind    string `json:"kind"`
	Bitrate int    `json:"bitrate,omitempty"`
}

type audioOutputSpecDTO struct {
	Codec   string           `json:"codec"`
	Quality *audioQualityDTO `json:"quality,omitempty"`
}

type desiredProfileDTO struct {
	Lossless *audioOutputSpecDTO `json:"lossless,omitempty"`
	Encoded  *audioOutputSpecDTO `json:"encoded,omitempty"`
}

// opaqueDraftRequest captures the raw JSON body of non-conversion draft PUTs
// so the target Task validates its own schema version and fields.
type opaqueDraftRequest json.RawMessage

// UnmarshalJSON copies the raw JSON bytes unchanged.
func (o *opaqueDraftRequest) UnmarshalJSON(data []byte) error {
	*o = append((*o)[:0], data...)
	return nil
}

type memberConclusionResponse struct {
	MemberID      string          `json:"member_id"`
	Excluded      bool            `json:"excluded"`
	Input         string          `json:"input"`
	HasComponents bool            `json:"has_components"`
	Task          json.RawMessage `json:"task,omitempty"`
	Category      string          `json:"category,omitempty"`
}

func toMemberConclusionResponse(c workset.MemberConclusion) memberConclusionResponse {
	return memberConclusionResponse{
		MemberID:      c.MemberID,
		Excluded:      c.Excluded,
		Input:         c.Input,
		HasComponents: c.HasComponents,
		Task:          c.Task,
		Category:      c.Category,
	}
}

func toMemberConclusionResponses(in []workset.MemberConclusion) []memberConclusionResponse {
	if in == nil {
		return nil
	}
	out := make([]memberConclusionResponse, len(in))
	for i, item := range in {
		out[i] = toMemberConclusionResponse(item)
	}
	return out
}

type taskClassifierSnapshot struct {
	Tags []string `json:"tags"`
	Hash string   `json:"hash"`
}

type revisionResponse struct {
	PlanID         string                     `json:"plan_id"`
	RevisionIndex  int                        `json:"revision_index"`
	CreatedAt      string                     `json:"created_at"`
	RootPath       string                     `json:"root_path"`
	SnapshotToken  string                     `json:"snapshot_token"`
	Status         string                     `json:"status"`
	Summary        planSummaryResponse        `json:"summary"`
	Task           taskEnvelopeResponse       `json:"task"`
	Counts         revisionCountsResponse     `json:"counts"`
	CategoryCounts map[string]int             `json:"category_counts"`
	CategoryTotal  *int                       `json:"category_total,omitempty"`
	Members        []memberConclusionResponse `json:"members"`
	Execution      *executionRefResponse      `json:"execution"`
}

// ==================== Generation DTOs ====================

type generationReplayRevisionResponse struct {
	Created  bool                     `json:"created"`
	Revision *currentRevisionResponse `json:"revision"`
}

type generationStartResponse struct {
	Created    bool                   `json:"created"`
	Generation generationViewResponse `json:"generation"`
}

// ==================== Execution DTOs ====================

type executionStartRequest struct {
	MemberIDs          []string `json:"member_ids"`
	AllExceptMemberIDs []string `json:"all_except_member_ids"`
}

type executionStartResponse struct {
	Created   bool                  `json:"created"`
	Execution executionViewResponse `json:"execution"`
}

type executionRefResponse struct {
	ExecutionID  string `json:"execution_id"`
	PlanID       string `json:"plan_id"`
	Status       string `json:"status"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	FinishedAt   string `json:"finished_at,omitempty"`
}

type executionProgressResponse struct {
	ExecutionID         string          `json:"execution_id"`
	PlanID              string          `json:"plan_id"`
	Status              string          `json:"status"`
	Options             json.RawMessage `json:"options,omitempty"`
	TotalComponents     int             `json:"total_components"`
	CompletedComponents int             `json:"completed_components"`
	TotalOperations     int             `json:"total_operations"`
	CompletedOperations int             `json:"completed_operations"`
	CurrentRoot         string          `json:"current_root"`
	CurrentComponentID  string          `json:"current_component_id"`
	CurrentPhase        string          `json:"current_phase"`
}

type executionMemberSummaryResponse struct {
	MemberID            string `json:"member_id"`
	RootPath            string `json:"root_path"`
	Status              string `json:"status"`
	TotalComponents     int    `json:"total_components"`
	CompletedComponents int    `json:"completed_components"`
	TotalOperations     int    `json:"total_operations"`
	CompletedOperations int    `json:"completed_operations"`
	ErrorMessage        string `json:"error_message"`
	InventorySyncError  string `json:"inventory_sync_error"`
}

type executionMemberCountsResponse struct {
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Canceled  int `json:"canceled"`
	Running   int `json:"running"`
	Pending   int `json:"pending"`
}

type executionMembersResponse struct {
	Counts         executionMemberCountsResponse    `json:"counts"`
	Attention      []executionMemberSummaryResponse `json:"attention"`
	AttentionTotal int                              `json:"attention_total"`
}

type executionComponentResponse struct {
	TaskResult         json.RawMessage `json:"task_result,omitempty"`
	KeptDecisions      json.RawMessage `json:"kept_decisions"`
	ComponentIndex     int             `json:"component_index"`
	ComponentID        string          `json:"component_id"`
	RootPath           string          `json:"root_path"`
	Partition          string          `json:"partition"`
	Status             string          `json:"status"`
	Stage              string          `json:"stage,omitempty"`
	Operations         int             `json:"operations"`
	CompletedOps       int             `json:"completed_operations"`
	Committed          []string        `json:"committed"`
	Removed            []string        `json:"removed"`
	Remaining          []string        `json:"remaining"`
	Recovery           []string        `json:"recovery"`
	ErrorCode          string          `json:"error_code,omitempty"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	InventorySynced    bool            `json:"inventory_synced"`
	InventorySyncError string          `json:"inventory_sync_error,omitempty"`
}

type executionViewResponse struct {
	ExecutionID         string                          `json:"execution_id"`
	WorksetID           string                          `json:"workset_id"`
	OperationType       string                          `json:"operation_type"`
	PlanID              string                          `json:"plan_id"`
	Status              string                          `json:"status"`
	Options             json.RawMessage                 `json:"options,omitempty"`
	TotalComponents     int                             `json:"total_components"`
	CompletedComponents int                             `json:"completed_components"`
	TotalOperations     int                             `json:"total_operations"`
	CompletedOperations int                             `json:"completed_operations"`
	CurrentRoot         string                          `json:"current_root"`
	CurrentComponentID  string                          `json:"current_component_id"`
	CurrentPhase        string                          `json:"current_phase"`
	Components          []executionComponentResponse    `json:"components"`
	Members             *executionMembersResponse       `json:"members,omitempty"`
	Member              *executionMemberSummaryResponse `json:"member,omitempty"`
	SelectedMemberIDs   []string                        `json:"selected_member_ids,omitempty"`
	ErrorCode           string                          `json:"error_code"`
	ErrorMessage        string                          `json:"error_message"`
	StartedAt           string                          `json:"started_at"`
	FinishedAt          string                          `json:"finished_at"`
	CreatedAt           string                          `json:"created_at"`
}

// MarshalJSON leaves `components` out of a members view; every other view keeps
// it, matching workset.ExecutionView.
func (v executionViewResponse) MarshalJSON() ([]byte, error) {
	type plain executionViewResponse
	if v.Members == nil {
		return json.Marshal(plain(v))
	}
	return json.Marshal(struct {
		plain

		Components []executionComponentResponse `json:"components,omitempty"`
	}{plain: plain(v)})
}

func toExecutionRefResponse(r *workset.ExecutionRef) *executionRefResponse {
	if r == nil {
		return nil
	}
	return &executionRefResponse{
		ExecutionID:  r.ExecutionID,
		PlanID:       r.PlanID,
		Status:       r.Status,
		ErrorCode:    r.ErrorCode,
		ErrorMessage: r.ErrorMessage,
		FinishedAt:   r.FinishedAt,
	}
}

func toExecutionProgressResponse(p *workset.ExecutionProgress) *executionProgressResponse {
	if p == nil {
		return nil
	}
	return &executionProgressResponse{
		ExecutionID:         p.ExecutionID,
		PlanID:              p.PlanID,
		Status:              p.Status,
		Options:             p.Options,
		TotalComponents:     p.TotalComponents,
		CompletedComponents: p.CompletedComponents,
		TotalOperations:     p.TotalOperations,
		CompletedOperations: p.CompletedOperations,
		CurrentRoot:         p.CurrentRoot,
		CurrentComponentID:  p.CurrentComponentID,
		CurrentPhase:        p.CurrentPhase,
	}
}

func toExecutionMemberSummaryResponse(m workset.MemberSummary) executionMemberSummaryResponse {
	return executionMemberSummaryResponse{
		MemberID:            m.MemberID,
		RootPath:            m.RootPath,
		Status:              m.Status,
		TotalComponents:     m.TotalComponents,
		CompletedComponents: m.CompletedComponents,
		TotalOperations:     m.TotalOperations,
		CompletedOperations: m.CompletedOperations,
		ErrorMessage:        m.ErrorMessage,
		InventorySyncError:  m.InventorySyncError,
	}
}

func toExecutionMemberCountsResponse(c workset.MemberCounts) executionMemberCountsResponse {
	return executionMemberCountsResponse{
		Succeeded: c.Succeeded,
		Failed:    c.Failed,
		Canceled:  c.Canceled,
		Running:   c.Running,
		Pending:   c.Pending,
	}
}

func toExecutionComponentResponse(c workset.ExecutionComponentView) executionComponentResponse {
	return executionComponentResponse{
		TaskResult:         c.TaskResult,
		KeptDecisions:      c.KeptDecisions,
		ComponentIndex:     c.ComponentIndex,
		ComponentID:        c.ComponentID,
		RootPath:           c.RootPath,
		Partition:          c.Partition,
		Status:             c.Status,
		Stage:              c.Stage,
		Operations:         c.Operations,
		CompletedOps:       c.CompletedOps,
		Committed:          c.Committed,
		Removed:            c.Removed,
		Remaining:          c.Remaining,
		Recovery:           c.Recovery,
		ErrorCode:          c.ErrorCode,
		ErrorMessage:       c.ErrorMessage,
		InventorySynced:    c.InventorySynced,
		InventorySyncError: c.InventorySyncError,
	}
}

func toExecutionViewResponse(v *workset.ExecutionView) executionViewResponse {
	var components []executionComponentResponse
	if v.Components != nil {
		components = make([]executionComponentResponse, len(v.Components))
		for i, c := range v.Components {
			components[i] = toExecutionComponentResponse(c)
		}
	}
	var members *executionMembersResponse
	if v.Members != nil {
		var attention []executionMemberSummaryResponse
		if v.Members.Attention != nil {
			attention = make([]executionMemberSummaryResponse, len(v.Members.Attention))
			for i, item := range v.Members.Attention {
				attention[i] = toExecutionMemberSummaryResponse(item)
			}
		}
		members = &executionMembersResponse{
			Counts:         toExecutionMemberCountsResponse(v.Members.Counts),
			Attention:      attention,
			AttentionTotal: v.Members.AttentionTotal,
		}
	}
	var member *executionMemberSummaryResponse
	if v.Member != nil {
		m := toExecutionMemberSummaryResponse(*v.Member)
		member = &m
	}
	return executionViewResponse{
		ExecutionID:         v.ExecutionID,
		WorksetID:           v.WorksetID,
		OperationType:       v.OperationType,
		PlanID:              v.PlanID,
		Status:              v.Status,
		Options:             v.Options,
		TotalComponents:     v.TotalComponents,
		CompletedComponents: v.CompletedComponents,
		TotalOperations:     v.TotalOperations,
		CompletedOperations: v.CompletedOperations,
		CurrentRoot:         v.CurrentRoot,
		CurrentComponentID:  v.CurrentComponentID,
		CurrentPhase:        v.CurrentPhase,
		Components:          components,
		Members:             members,
		Member:              member,
		SelectedMemberIDs:   v.SelectedMemberIDs,
		ErrorCode:           v.ErrorCode,
		ErrorMessage:        v.ErrorMessage,
		StartedAt:           v.StartedAt,
		FinishedAt:          v.FinishedAt,
		CreatedAt:           v.CreatedAt,
	}
}

// ==================== SSE DTOs & Emitters ====================

type generationSnapshotEvent struct {
	GenerationID   string `json:"generation_id"`
	WorksetID      string `json:"workset_id"`
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

type generationProgressEvent struct {
	GenerationID   string `json:"generation_id"`
	TotalRoots     int    `json:"total_roots"`
	CompletedRoots int    `json:"completed_roots"`
	CurrentRoot    string `json:"current_root"`
	ErrorCount     int    `json:"error_count"`
}

type generationCompletedEvent struct {
	GenerationID string `json:"generation_id"`
	RevisionID   string `json:"revision_id"`
}

type generationFailedEvent struct {
	GenerationID string `json:"generation_id"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

type generationStoppedEvent struct {
	GenerationID string `json:"generation_id"`
}

type sessionStatusEvent struct {
	Status string `json:"status"`
}

type executionMemberEvent struct {
	Member         executionMemberSummaryResponse `json:"member"`
	Counts         executionMemberCountsResponse  `json:"counts"`
	AttentionTotal int                            `json:"attention_total"`
}

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

type executionSucceededEvent struct {
	ExecutionID string `json:"execution_id"`
	PlanID      string `json:"plan_id"`
}

type executionFailedEvent struct {
	ExecutionID  string `json:"execution_id"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

type executionStoppedEvent struct {
	ExecutionID string `json:"execution_id"`
}

func openSSEStream(w http.ResponseWriter) (*SSEWriter, bool) {
	sw, err := newSSEWriter(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "streaming not supported")
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	return sw, true
}

func writeSSEError(sw *SSEWriter, err error, notFoundCode string) {
	if werr, ok := workset.AsError(err); ok && werr.Code == notFoundCode {
		_ = sw.Send("error", errorResponse{Code: notFoundCode, Message: werr.Message})
		return
	}
	_ = sw.Send("error", errorResponse{Code: "INTERNAL", Message: "streaming failed"})
}

func emitGenerationEvent(sw *SSEWriter, event string, data any) error {
	switch payload := data.(type) {
	case workset.GenerationSnapshot:
		return sw.Send(event, generationSnapshotEvent{
			GenerationID:   payload.GenerationID,
			WorksetID:      payload.WorksetID,
			Status:         payload.Status,
			TotalRoots:     payload.TotalRoots,
			CompletedRoots: payload.CompletedRoots,
			CurrentRoot:    payload.CurrentRoot,
			ErrorCount:     payload.ErrorCount,
			RevisionID:     payload.RevisionID,
			ErrorCode:      payload.ErrorCode,
			ErrorMessage:   payload.ErrorMessage,
			StartedAt:      payload.StartedAt,
			FinishedAt:     payload.FinishedAt,
			CreatedAt:      payload.CreatedAt,
		})
	case workset.GenerationProgressEvent:
		return sw.Send(event, generationProgressEvent{
			GenerationID:   payload.GenerationID,
			TotalRoots:     payload.TotalRoots,
			CompletedRoots: payload.CompletedRoots,
			CurrentRoot:    payload.CurrentRoot,
			ErrorCount:     payload.ErrorCount,
		})
	case workset.GenerationCompletedEvent:
		return sw.Send(event, generationCompletedEvent{
			GenerationID: payload.GenerationID,
			RevisionID:   payload.RevisionID,
		})
	case workset.GenerationFailedEvent:
		return sw.Send(event, generationFailedEvent{
			GenerationID: payload.GenerationID,
			ErrorCode:    payload.ErrorCode,
			ErrorMessage: payload.ErrorMessage,
		})
	case workset.GenerationStoppedEvent:
		return sw.Send(event, generationStoppedEvent{GenerationID: payload.GenerationID})
	case workset.SessionStatusEvent:
		return sw.Send(event, sessionStatusEvent{Status: payload.Status})
	default:
		return fmt.Errorf("unexpected generation SSE payload %T for event %s", data, event)
	}
}

func emitExecutionEvent(sw *SSEWriter, event string, data any) error {
	switch payload := data.(type) {
	case *workset.ExecutionView:
		return sw.Send(event, toExecutionViewResponse(payload))
	case workset.ExecutionComponentView:
		return sw.Send(event, toExecutionComponentResponse(payload))
	case workset.MemberEvent:
		return sw.Send(event, executionMemberEvent{
			Member:         toExecutionMemberSummaryResponse(payload.Member),
			Counts:         toExecutionMemberCountsResponse(payload.Counts),
			AttentionTotal: payload.AttentionTotal,
		})
	case workset.ExecutionProgressEvent:
		return sw.Send(event, executionProgressEvent{
			ExecutionID:         payload.ExecutionID,
			Status:              payload.Status,
			TotalComponents:     payload.TotalComponents,
			CompletedComponents: payload.CompletedComponents,
			TotalOperations:     payload.TotalOperations,
			CompletedOperations: payload.CompletedOperations,
			CurrentRoot:         payload.CurrentRoot,
			CurrentComponentID:  payload.CurrentComponentID,
			CurrentPhase:        payload.CurrentPhase,
		})
	case workset.ExecutionSucceededEvent:
		return sw.Send(event, executionSucceededEvent{
			ExecutionID: payload.ExecutionID,
			PlanID:      payload.PlanID,
		})
	case workset.ExecutionFailedEvent:
		return sw.Send(event, executionFailedEvent{
			ExecutionID:  payload.ExecutionID,
			ErrorCode:    payload.ErrorCode,
			ErrorMessage: payload.ErrorMessage,
		})
	case workset.ExecutionStoppedEvent:
		return sw.Send(event, executionStoppedEvent{ExecutionID: payload.ExecutionID})
	case workset.SessionStatusEvent:
		return sw.Send(event, sessionStatusEvent{Status: payload.Status})
	default:
		return fmt.Errorf("unexpected execution SSE payload %T for event %s", data, event)
	}
}
