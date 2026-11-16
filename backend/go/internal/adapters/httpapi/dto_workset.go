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

// Leaf wire types that match their domain type field for field are defined
// from it, so a domain change reaches the wire schema (docs/api/schema,
// task contract:check) and conversions are plain type conversions.
type memberConclusionResponse workset.MemberConclusion

func toMemberConclusionResponses(in []workset.MemberConclusion) []memberConclusionResponse {
	out := make([]memberConclusionResponse, len(in))
	for i, item := range in {
		out[i] = memberConclusionResponse(item)
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

type executionRefResponse workset.ExecutionRef

type executionProgressResponse workset.ExecutionProgress

type executionMemberSummaryResponse workset.MemberSummary

type executionMemberCountsResponse workset.MemberCounts

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
		Committed:          orEmpty(c.Committed),
		Removed:            orEmpty(c.Removed),
		Remaining:          orEmpty(c.Remaining),
		Recovery:           orEmpty(c.Recovery),
		ErrorCode:          c.ErrorCode,
		ErrorMessage:       c.ErrorMessage,
		InventorySynced:    c.InventorySynced,
		InventorySyncError: c.InventorySyncError,
	}
}

func toExecutionViewResponse(v *workset.ExecutionView) executionViewResponse {
	components := make([]executionComponentResponse, len(v.Components))
	for i, c := range v.Components {
		components[i] = toExecutionComponentResponse(c)
	}
	var members *executionMembersResponse
	if v.Members != nil {
		attention := make([]executionMemberSummaryResponse, len(v.Members.Attention))
		for i, item := range v.Members.Attention {
			attention[i] = executionMemberSummaryResponse(item)
		}
		members = &executionMembersResponse{
			Counts:         executionMemberCountsResponse(v.Members.Counts),
			Attention:      attention,
			AttentionTotal: v.Members.AttentionTotal,
		}
	}
	var member *executionMemberSummaryResponse
	if v.Member != nil {
		member = (*executionMemberSummaryResponse)(v.Member)
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

type generationProgressEvent workset.GenerationProgressEvent

type generationCompletedEvent workset.GenerationCompletedEvent

type generationFailedEvent workset.GenerationFailedEvent

type generationStoppedEvent workset.GenerationStoppedEvent

type sessionStatusEvent workset.SessionStatusEvent

type executionMemberEvent struct {
	Member         executionMemberSummaryResponse `json:"member"`
	Counts         executionMemberCountsResponse  `json:"counts"`
	AttentionTotal int                            `json:"attention_total"`
}

type executionProgressEvent workset.ExecutionProgressEvent

type executionSucceededEvent workset.ExecutionSucceededEvent

type executionFailedEvent workset.ExecutionFailedEvent

type executionStoppedEvent workset.ExecutionStoppedEvent

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
		return sw.Send(event, generationViewResponse(payload))
	case workset.GenerationProgressEvent:
		return sw.Send(event, generationProgressEvent(payload))
	case workset.GenerationCompletedEvent:
		return sw.Send(event, generationCompletedEvent(payload))
	case workset.GenerationFailedEvent:
		return sw.Send(event, generationFailedEvent(payload))
	case workset.GenerationStoppedEvent:
		return sw.Send(event, generationStoppedEvent(payload))
	case workset.SessionStatusEvent:
		return sw.Send(event, sessionStatusEvent(payload))
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
			Member:         executionMemberSummaryResponse(payload.Member),
			Counts:         executionMemberCountsResponse(payload.Counts),
			AttentionTotal: payload.AttentionTotal,
		})
	case workset.ExecutionProgressEvent:
		return sw.Send(event, executionProgressEvent(payload))
	case workset.ExecutionSucceededEvent:
		return sw.Send(event, executionSucceededEvent(payload))
	case workset.ExecutionFailedEvent:
		return sw.Send(event, executionFailedEvent(payload))
	case workset.ExecutionStoppedEvent:
		return sw.Send(event, executionStoppedEvent(payload))
	case workset.SessionStatusEvent:
		return sw.Send(event, sessionStatusEvent(payload))
	default:
		return fmt.Errorf("unexpected execution SSE payload %T for event %s", data, event)
	}
}
