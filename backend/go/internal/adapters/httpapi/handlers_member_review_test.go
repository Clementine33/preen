package httpapi //nolint:testpackage // shared HTTP fixture constructs the real service and repository

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestMemberReviewHTTP(t *testing.T) {
	h, repo := newWorksetServer(t)
	libID := seedLibrary(t, repo)
	seedMember(t, repo, "albumA")
	wsID := createRecord(t, h, libID, "member-review-http")
	svc := workset.NewService(repo, 1, 1, []workset.Task{&conversion.Task{}}, nil, nil)
	seedRevision(t, repo, svc, libID, wsID, "member-plan")
	members, err := repo.ListWorksetMembers(wsID)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/worksets/" + wsID + "/operations/conversion/revisions/"
	response := req(t, h, http.MethodGet, base+"member-plan/members/"+members[0].MemberID, testToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("member review = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		revisionMemberResponse

		PlanID string            `json:"plan_id"`
		Roots  []json.RawMessage `json:"roots"`
		Task   struct {
			Kind    string `json:"kind"`
			Payload struct {
				Components []json.RawMessage `json:"components"`
			} `json:"payload"`
		} `json:"task"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.PlanID != "member-plan" || body.MemberID != members[0].MemberID || body.Conclusion.Input != "none" ||
		body.Effective == nil ||
		body.Sources["mode"] != "common" ||
		body.Roots == nil ||
		body.Task.Payload.Components == nil ||
		len(body.Task.Payload.Components) != 0 ||
		body.Task.Kind != "conversion" {
		t.Fatalf("member review shape = %s", response.Body.String())
	}
	// The list gets conclusions only; settings and components stay at the member boundary.
	detail := req(t, h, http.MethodGet, base+"member-plan", testToken, nil)
	var light map[string]json.RawMessage
	if detail.Code != http.StatusOK || json.Unmarshal(detail.Body.Bytes(), &light) != nil {
		t.Fatalf("revision detail = %d %s", detail.Code, detail.Body.String())
	}
	fields := []string{
		"plan_id",
		"revision_index",
		"created_at",
		"root_path",
		"snapshot_token",
		"status",
		"summary",
		"counts",
		"execution",
		"task",
		"members",
	}
	if len(light) != len(fields) {
		t.Fatalf("light revision fields: %s", detail.Body.String())
	}
	for _, field := range fields {
		if _, exists := light[field]; !exists {
			t.Fatalf("light revision missing %s", field)
		}
	}
	for _, field := range []string{"roots", "component_roots"} {
		if _, exists := light[field]; exists {
			t.Fatalf("light revision contains %s", field)
		}
	}
	var task struct {
		Payload map[string]json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(light["task"], &task); err != nil {
		t.Fatal(err)
	}
	if _, exists := task.Payload["components"]; exists {
		t.Fatal("light task contains components")
	}
	var conclusions []map[string]json.RawMessage
	if err := json.Unmarshal(light["members"], &conclusions); err != nil {
		t.Fatal(err)
	}
	if len(conclusions) != 1 || string(conclusions[0]["input"]) != `"none"` {
		t.Fatalf("light members are not conclusions: %s", light["members"])
	}
	for _, field := range []string{"effective", "sources", "conclusion", "folder_path", "member_name"} {
		if _, exists := conclusions[0][field]; exists {
			t.Fatalf("light member contains %s", field)
		}
	}
	for _, c := range []struct{ path, code string }{
		{"member-plan/members/foreign", "MEMBER_NOT_FOUND"},
		{"old-plan/members/" + members[0].MemberID, "REVISION_NOT_FOUND"},
	} {
		missing := req(t, h, http.MethodGet, base+c.path, testToken, nil)
		if missing.Code != http.StatusNotFound || !containsCode(missing.Body.Bytes(), c.code) {
			t.Fatalf("%s = %d %s", c.path, missing.Code, missing.Body.String())
		}
	}
}
