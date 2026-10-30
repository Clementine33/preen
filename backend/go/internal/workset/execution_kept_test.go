package workset_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func keptExecutionFixture(t *testing.T) *execFixture {
	t.Helper()
	f := newExecFixture(t)
	f.seedRevision("plan-kept", seedComponent{
		id: "kept", member: "albumA", partition: reconcile.PartitionMatched,
		variants: []reconcile.VariantDecision{{Stem: "00", Decisions: []reconcile.FileDecision{
			{
				Path:       "/music/albumA/00.mp3",
				Resolution: "keep",
				ReasonCode: "KEEP_ENCODED_SATISFIED",
				Message:    "satisfied",
			},
			{Path: "/music/albumA/00.flac", Resolution: "remove", ReasonCode: "OBSOLETE"},
		}}, {Stem: "01", Decisions: []reconcile.FileDecision{
			{Path: "/music/albumA/01.wav", Resolution: "keep", ReasonCode: "UNMET_TARGET"},
		}}},
	}, seedComponent{id: "empty", member: "albumB", partition: reconcile.PartitionUnmatched})
	return f
}

func assertExecutionKept(t *testing.T, component any, empty bool) {
	t.Helper()
	raw, err := json.Marshal(component)
	if err != nil {
		t.Fatal(err)
	}
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatal(err)
	}
	var got []reconcile.FileDecision
	if err := json.Unmarshal(entry["kept_decisions"], &got); err != nil {
		t.Fatalf("kept_decisions missing or invalid: %s: %v", raw, err)
	}
	if got == nil {
		t.Fatalf("kept_decisions must be an array: %s", raw)
	}
	want := []reconcile.FileDecision{}
	if !empty {
		want = []reconcile.FileDecision{
			{
				Path:       "/music/albumA/00.mp3",
				Resolution: "keep",
				ReasonCode: "KEEP_ENCODED_SATISFIED",
				Message:    "satisfied",
			},
			{Path: "/music/albumA/01.wav", Resolution: "keep", ReasonCode: "UNMET_TARGET"},
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("kept decisions = %+v, want %+v", got, want)
	}
}

func TestExecutionHTTPIncludesFrozenKeptDecisions(t *testing.T) {
	f := keptExecutionFixture(t)
	handler := httpapi.NewServer(httpapi.Dependencies{WorksetService: f.svc, Token: "test"})
	opPath := "/api/v1/worksets/" + f.worksetID + "/operations/conversion"
	request := func(method, path string, status int) map[string]json.RawMessage {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer test")
		req.Header.Set("If-Match", strconv.Itoa(f.operation().Version))
		req.Header.Set("Idempotency-Key", "kept")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("%s %s = %d: %s", method, path, response.Code, response.Body)
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload
	}
	start := request(http.MethodPost, opPath+"/revisions/plan-kept/executions", http.StatusAccepted)
	var session workset.ExecutionView
	if err := json.Unmarshal(start["execution"], &session); err != nil {
		t.Fatal(err)
	}
	assertExecutionKept(t, session.Components[0], false)
	assertExecutionKept(t, session.Components[1], true)
	replay := request(http.MethodPost, opPath+"/revisions/plan-kept/executions", http.StatusOK)
	if err := json.Unmarshal(replay["execution"], &session); err != nil {
		t.Fatal(err)
	}
	assertExecutionKept(t, session.Components[0], false)
	detailPath := opPath + "/executions/" + session.ExecutionID
	for _, page := range []struct {
		query string
		empty bool
	}{{"?components_from=0&components_limit=1", false}, {"?components_from=1&components_limit=1", true}} {
		detail := request(http.MethodGet, detailPath+page.query, http.StatusOK)
		var components []map[string]json.RawMessage
		if err := json.Unmarshal(detail["components"], &components); err != nil {
			t.Fatal(err)
		}
		if len(components) != 1 {
			t.Fatalf("page components = %s", detail["components"])
		}
		assertExecutionKept(t, components[0], page.empty)
	}
	canceled := request(http.MethodPost, detailPath+"/cancel", http.StatusOK)
	var components []map[string]json.RawMessage
	if err := json.Unmarshal(canceled["components"], &components); err != nil {
		t.Fatal(err)
	}
	assertExecutionKept(t, components[0], false)
	assertExecutionKept(t, components[1], true)
}

func TestExecutionStreamRetainsFrozenKeptDecisions(t *testing.T) {
	f := keptExecutionFixture(t)
	started := f.mustStart("plan-kept", "kept")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var snapshotSeen, componentSeen bool
	err := f.svc.SubscribeExecution(
		ctx,
		f.worksetID,
		"conversion",
		started.ExecutionID,
		func(event string, data any) error {
			switch event {
			case workset.ExecutionEventSnapshot:
				snapshotSeen = true
				snapshot, ok := data.(*workset.ExecutionView)
				if !ok {
					t.Fatalf("snapshot type = %T", data)
				}
				assertExecutionKept(t, snapshot.Components[0], false)
				assertExecutionKept(t, snapshot.Components[1], true)
				// A result arriving after this connection's snapshot must carry the same
				// frozen decisions, even though the result itself has no file decisions.
				return f.repo.SaveExecutionComponentResult(started.ExecutionID, workset.ExecutionComponentResult{
					ComponentIndex: 0,
					Status:         workset.ExecComponentFailed,
					ResultJSON:     `{"error_code":"COMPONENT_FAILED"}`,
				}, workset.ExecutionPosition{})
			case workset.ExecutionEventComponent:
				componentSeen = true
				assertExecutionKept(t, data, false)
				cancel()
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshotSeen || !componentSeen {
		t.Fatalf("snapshot = %v, component = %v", snapshotSeen, componentSeen)
	}
	if _, cancelErr := f.svc.CancelExecution(
		t.Context(),
		f.worksetID,
		"conversion",
		started.ExecutionID,
	); cancelErr != nil {
		t.Fatal(cancelErr)
	}
	// Reconnection reconstructs the persisted failed component plus its plan facts.
	err = f.svc.SubscribeExecution(
		t.Context(),
		f.worksetID,
		"conversion",
		started.ExecutionID,
		func(event string, data any) error {
			if event == workset.ExecutionEventSnapshot {
				snapshot, ok := data.(*workset.ExecutionView)
				if !ok {
					t.Fatalf("snapshot type = %T", data)
				}
				if snapshot.Components[0].Status != workset.ExecComponentFailed {
					t.Fatalf("reconnect status = %s", snapshot.Components[0].Status)
				}
				assertExecutionKept(t, snapshot.Components[0], false)
				assertExecutionKept(t, snapshot.Components[1], true)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}
