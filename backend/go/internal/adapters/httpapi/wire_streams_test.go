package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	"github.com/Clementine33/preen/backend/internal/testutil/wirecheck"
	"github.com/Clementine33/preen/backend/internal/workset"
)

type streamWireService struct {
	workset.Service

	event   string
	payload any
	failure error
}

func (s streamWireService) Subscribe(_ context.Context, _, _, _ string, emit func(string, any) error) error {
	if s.failure != nil {
		return s.failure
	}
	return emit(s.event, s.payload)
}
func (s streamWireService) SubscribeExecution(ctx context.Context, a, b, c string, emit func(string, any) error) error {
	return s.Subscribe(ctx, a, b, c, emit)
}

func (s streamWireService) SubscribeExecutionMembers(
	ctx context.Context,
	a, b, c string,
	emit func(string, any) error,
) error {
	return s.Subscribe(ctx, a, b, c, emit)
}

// Every event is driven through the real router, adapter projection, framing and
// schema check. The fake supplies domain events rather than encoded wire DTOs.
func TestWireGenerationEvents(t *testing.T) {
	for _, tc := range []struct {
		event   string
		payload any
	}{
		{"session_snapshot", workset.GenerationSnapshot{OperationType: "rename", Status: "queued"}},
		{"progress", workset.GenerationProgressEvent{}},
		{"completed", workset.GenerationCompletedEvent{}},
		{"failed", workset.GenerationFailedEvent{}},
		{"canceled", workset.GenerationStoppedEvent{}},
		{"interrupted", workset.GenerationStoppedEvent{}},
		{"message", workset.SessionStatusEvent{Status: "unexpected"}},
	} {
		t.Run(tc.event, func(t *testing.T) {
			body := assertWireStream(
				t,
				"planning-sessions/g/events",
				streamWireService{event: tc.event, payload: tc.payload},
			)
			if tc.event == "session_snapshot" && !strings.Contains(body, `"operation_type":"rename"`) {
				t.Fatalf("snapshot lacks operation_type: %s", body)
			}
		})
	}
	assertWireStream(
		t,
		"planning-sessions/missing/events",
		streamWireService{failure: errors.New("subscription failed")},
	)
}

func TestWireExecutionEvents(t *testing.T) {
	for _, tc := range []struct {
		event   string
		payload any
	}{
		{"execution_snapshot", &workset.ExecutionView{OperationType: "conversion", Status: "queued"}},
		{"component", workset.ExecutionComponentView{Status: "pending", KeptDecisions: json.RawMessage(`[]`)}},
		{"member", workset.MemberEvent{Member: workset.MemberSummary{Status: "pending"}}},
		{"progress", workset.ExecutionProgressEvent{Status: "running"}},
		{"succeeded", workset.ExecutionSucceededEvent{}},
		{"failed", workset.ExecutionFailedEvent{}},
		{"canceled", workset.ExecutionStoppedEvent{}},
		{"interrupted", workset.ExecutionStoppedEvent{}},
		{"message", workset.SessionStatusEvent{Status: "unexpected"}},
	} {
		t.Run(tc.event, func(t *testing.T) {
			tail := "executions/e/events"
			if tc.event == "member" {
				tail += "?view=members"
			}
			assertWireStream(t, tail, streamWireService{event: tc.event, payload: tc.payload})
		})
	}
	assertWireStream(t, "executions/missing/events", streamWireService{failure: errors.New("subscription failed")})
	assertWireStream(t, "executions/e/events?view=members", streamWireService{
		event: "execution_snapshot",
		payload: &workset.ExecutionView{
			OperationType: "conversion",
			Status:        "queued",
			Members:       &workset.ExecutionMembers{},
		},
	})
}

func assertWireStream(t *testing.T, tail string, service streamWireService) string {
	t.Helper()
	handler := wirecheck.Handler(t, httpapi.NewServer(httpapi.Dependencies{WorksetService: service}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/api/v1/worksets/w/operations/conversion/"+tail, nil),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	return response.Body.String()
}
