package workset_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func (f *execFixture) saveResult(execID string, index int, status, resultJSON string, next workset.ExecutionPosition) {
	f.t.Helper()
	if err := f.repo.SaveExecutionComponentResult(execID, workset.ExecutionComponentResult{
		ComponentIndex: index, Status: status, ResultJSON: resultJSON,
	}, next); err != nil {
		f.t.Fatal(err)
	}
}

func (f *execFixture) membersOf(execID string) *workset.ExecutionView {
	f.t.Helper()
	view, err := f.svc.GetExecution(
		f.t.Context(), f.worksetID, workset.OperationTypeConversion, execID, workset.ExecutionRead{Members: true},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return view
}

func TestExecutionMembersViewSummarizesResults(t *testing.T) {
	f := keptExecutionFixture(t)
	started := f.mustStart("plan-kept", "m1")
	a, b := f.members["albumA"], f.members["albumB"]

	idle := f.membersOf(started.ExecutionID)
	if got := idle.Members.Counts; got != (workset.MemberCounts{Pending: 2}) || idle.Members.AttentionTotal != 0 ||
		idle.Members.Attention == nil || len(idle.Components) != 0 {
		t.Fatalf("idle members = %+v components=%d", idle.Members, len(idle.Components))
	}

	f.saveResult(
		started.ExecutionID, 0, workset.ExecComponentFailed, `{"error_code":"BOOM"}`, workset.ExecutionPosition{},
	)
	f.saveResult(started.ExecutionID, 1, workset.ExecComponentSucceeded,
		`{"inventory_sync_error":"stat failed"}`, workset.ExecutionPosition{})
	view := f.membersOf(started.ExecutionID)
	got := view.Members.Counts
	if got != (workset.MemberCounts{Failed: 1, Succeeded: 1}) || view.Members.AttentionTotal != 2 {
		t.Fatalf("counts = %+v total %d", got, view.Members.AttentionTotal)
	}
	attention := view.Members.Attention
	if len(attention) != 2 || attention[0].MemberID != a.MemberID || attention[1].MemberID != b.MemberID {
		t.Fatalf("attention order = %+v", attention)
	}
	first := attention[0]
	if first.Status != "failed" || first.ErrorMessage != "BOOM" || first.RootPath != a.FolderPath ||
		first.TotalComponents != 1 || first.CompletedComponents != 1 {
		t.Fatalf("member A = %+v", first)
	}
	if attention[1].Status != "succeeded" || attention[1].InventorySyncError != "stat failed" {
		t.Fatalf("member B = %+v", attention[1])
	}
	raw, _ := json.Marshal(view)
	var wire map[string]json.RawMessage
	_ = json.Unmarshal(raw, &wire)
	if _, has := wire["components"]; has || wire["members"] == nil {
		t.Fatalf("wire = %s", raw)
	}
}

func TestExecutionMembersRunningWinsWhileLive(t *testing.T) {
	f := keptExecutionFixture(t)
	started := f.mustStart("plan-kept", "m2")
	if err := f.repo.UpdateExecutionPosition(started.ExecutionID, workset.ExecutionPosition{
		CurrentRoot: f.members["albumA"].FolderPath, CurrentComponentID: "kept",
	}); err != nil {
		t.Fatal(err)
	}
	view := f.membersOf(started.ExecutionID)
	if view.Members.Counts != (workset.MemberCounts{Running: 1, Pending: 1}) ||
		len(view.Members.Attention) != 1 || view.Members.Attention[0].Status != "running" {
		t.Fatalf("members = %+v", view.Members)
	}
	if _, err := f.svc.CancelExecution(
		t.Context(), f.worksetID, workset.OperationTypeConversion, started.ExecutionID,
	); err != nil {
		t.Fatal(err)
	}
	if got := f.membersOf(started.ExecutionID).Members.Counts; got.Running != 0 {
		t.Fatalf("a finished session has no running member: %+v", got)
	}
}

func TestExecutionMemberReadScopesComponents(t *testing.T) {
	f := keptExecutionFixture(t)
	started := f.mustStart("plan-kept", "m3")
	a, b := f.members["albumA"], f.members["albumB"]
	view, err := f.svc.GetExecution(
		t.Context(), f.worksetID, workset.OperationTypeConversion, started.ExecutionID,
		workset.ExecutionRead{MemberID: b.MemberID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Components) != 1 || view.Components[0].ComponentID != "empty" ||
		view.Member == nil || view.Member.MemberID != b.MemberID || view.Members != nil {
		t.Fatalf("view = %+v member=%+v", view.Components, view.Member)
	}
	view, err = f.svc.GetExecution(
		t.Context(), f.worksetID, workset.OperationTypeConversion, started.ExecutionID,
		workset.ExecutionRead{MemberID: a.MemberID},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertExecutionKept(t, view.Components[0], false)
	if _, err = f.svc.GetExecution(
		t.Context(), f.worksetID, workset.OperationTypeConversion, started.ExecutionID,
		workset.ExecutionRead{MemberID: "nope"},
	); !hasCode(err, "MEMBER_NOT_IN_EXECUTION") {
		t.Fatalf("unknown member err = %v", err)
	}
}

func TestExecutionMemberReadOutsideScopeIsNotFound(t *testing.T) {
	f := keptExecutionFixture(t)
	res, err := f.startExecution("plan-kept", "m4", "albumA")
	if err != nil {
		t.Fatal(err)
	}
	read := func(m workset.MemberView) error {
		_, readErr := f.svc.GetExecution(
			t.Context(), f.worksetID, workset.OperationTypeConversion, res.Execution.ExecutionID,
			workset.ExecutionRead{MemberID: m.MemberID},
		)
		return readErr
	}
	if err = read(f.members["albumA"]); err != nil {
		t.Fatal(err)
	}
	if err = read(f.members["albumB"]); !hasCode(err, "MEMBER_NOT_IN_EXECUTION") {
		t.Fatalf("out-of-scope err = %v", err)
	}
	if got := f.membersOf(res.Execution.ExecutionID).Members.Counts; got != (workset.MemberCounts{Pending: 1}) {
		t.Fatalf("scoped counts = %+v", got)
	}
}

func hasCode(err error, code string) bool {
	e, ok := workset.AsError(err)
	return ok && e.Code == code
}

func TestExecutionMembersStreamEmitsMemberEvents(t *testing.T) {
	f := keptExecutionFixture(t)
	started := f.mustStart("plan-kept", "m5")
	id := started.ExecutionID
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var names []string
	var members []workset.MemberEvent
	err := f.svc.SubscribeExecutionMembers(ctx, f.worksetID, workset.OperationTypeConversion, id,
		func(event string, data any) error {
			names = append(names, event)
			switch event {
			case workset.ExecutionEventSnapshot:
				view, _ := data.(*workset.ExecutionView)
				if view == nil || view.Members == nil || len(view.Components) != 0 {
					t.Fatalf("snapshot = %+v", view)
				}
				return f.repo.UpdateExecutionPosition(id, workset.ExecutionPosition{CurrentComponentID: "kept"})
			case workset.ExecutionEventComponent:
				t.Fatal("members mode must not emit component events")
			case workset.ExecutionEventMember:
				ev, _ := data.(workset.MemberEvent)
				members = append(members, ev)
				switch len(members) {
				case 1: // A is running; its result lands and the position moves to B
					f.saveResult(id, 0, workset.ExecComponentFailed, `{"error_message":"bad"}`,
						workset.ExecutionPosition{CurrentComponentID: "empty"})
				case 3:
					cancel()
				}
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 3 {
		t.Fatalf("members = %+v", members)
	}
	if members[0].Member.Status != "running" || members[0].Counts.Running != 1 {
		t.Fatalf("first = %+v", members[0])
	}
	m := members[1].Member
	if m.Status != "failed" || m.ErrorMessage != "bad" || m.MemberID != f.members["albumA"].MemberID {
		t.Fatalf("second = %+v", members[1])
	}
	last := members[2]
	if last.Member.MemberID != f.members["albumB"].MemberID || last.Member.Status != "running" ||
		last.Counts != (workset.MemberCounts{Failed: 1, Running: 1}) || last.AttentionTotal != 2 {
		t.Fatalf("third = %+v", last)
	}
	// Member events precede the progress of the same tick.
	want := []string{"execution_snapshot", "member", "progress", "member", "member"}
	if len(names) < len(want) {
		t.Fatalf("names = %v", names)
	}
	for i, n := range want {
		if names[i] != n {
			t.Fatalf("names = %v, want prefix %v", names, want)
		}
	}
}

func TestExecutionHTTPViewParameters(t *testing.T) {
	f := keptExecutionFixture(t)
	started := f.mustStart("plan-kept", "m6")
	handler := httpapi.NewServer(httpapi.Dependencies{WorksetService: f.svc, Token: "test"})
	get := func(query string) (int, map[string]json.RawMessage) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/worksets/"+f.worksetID+"/operations/conversion/executions/"+started.ExecutionID+query, nil)
		req.Header.Set("Authorization", "Bearer test")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		var payload map[string]json.RawMessage
		_ = json.Unmarshal(rec.Body.Bytes(), &payload)
		return rec.Code, payload
	}
	for _, query := range []string{"?view=other", "?view=members&member_id=x", "?view=", "?view=members&member_id="} {
		code, _ := get(query)
		want := http.StatusBadRequest
		if query == "?view=" || query == "?view=members&member_id=" {
			want = http.StatusOK
		}
		if code != want {
			t.Fatalf("%s = %d, want %d", query, code, want)
		}
	}
	code, body := get("?view=members")
	if code != http.StatusOK || body["members"] == nil || body["components"] != nil {
		t.Fatalf("view=members = %d %v", code, body)
	}
	code, body = get("?member_id=" + f.members["albumA"].MemberID)
	if code != http.StatusOK || body["member"] == nil || body["components"] == nil {
		t.Fatalf("member_id = %d %v", code, body)
	}
	code, body = get("?member_id=nope")
	if code != http.StatusNotFound || string(body["code"]) != `"MEMBER_NOT_IN_EXECUTION"` {
		t.Fatalf("unknown member = %d %v", code, body)
	}
	code, body = get("?components_from=1")
	if code != http.StatusOK || body["members"] != nil || len(body["components"]) < 3 {
		t.Fatalf("default = %d %v", code, body)
	}
}
