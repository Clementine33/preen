//nolint:testpackage // verifies private DTO marshalers against the public schemas
package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
	"github.com/Clementine33/preen/backend/internal/testutil/wirecheck"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
)

func TestWireCustomEncodings(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"reconcile_ComponentOutcome", reconcile.ComponentOutcome{Partition: reconcile.PartitionMatched, Status: reconcile.StatusOK}},
		{"reconcile_VariantDecision", reconcile.VariantDecision{}},
		{"workContentNodeResponse", workContentNodeResponse{Name: "directory"}},
		{"workContentNodeResponse", workContentNodeResponse{Name: "file", Kind: "aud"}},
		{"conversion_v1_executionViewResponse", executionViewResponse{
			OperationType: "conversion", Status: "queued", Components: []executionComponentResponse{},
		}},
		{"conversion_v1_executionViewResponse", executionViewResponse{
			OperationType: "conversion", Status: "queued",
			Members: &executionMembersResponse{Attention: []executionMemberSummaryResponse{}},
		}},
		{"conversion_v1_executionViewResponse", executionViewResponse{
			OperationType: "conversion", Status: "queued", Components: []executionComponentResponse{},
			Member: &executionMemberSummaryResponse{Status: "pending"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) { wirecheck.AssertSchema(t, tc.name, tc.value) })
	}
}

func TestWireThreeStateRequests(t *testing.T) {
	for _, raw := range []string{`{}`, `{"rating":null}`, `{"rating":7}`} {
		wirecheck.AssertSchema(t, "workUserDataPatchRequest", []byte(raw))
		var decoded workUserDataPatchRequest
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Rating.Set != (raw != `{}`) {
			t.Fatalf("rating presence lost for %s", raw)
		}
		if raw == `{"rating":7}` && (decoded.Rating.Value == nil || *decoded.Rating.Value != 7) {
			t.Fatal("rating value lost")
		}
	}
	for _, raw := range []string{`{}`, `{"default_library_id":null}`, `{"default_library_id":"library"}`} {
		wirecheck.AssertSchema(t, "preferencesUpdateRequest", []byte(raw))
		var decoded preferencesUpdateRequest
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.DefaultLibraryID.Set != (raw != `{}`) {
			t.Fatalf("library presence lost for %s", raw)
		}
	}
	for _, tc := range []struct{ name, raw string }{
		{"workUserDataPatchRequest", `{"rating":"7"}`},
		{"preferencesUpdateRequest", `{"default_library_id":17}`},
	} {
		if wirecheck.Validate(tc.name, []byte(tc.raw)) == nil {
			t.Errorf("accepted wrong type: %s", tc.raw)
		}
	}
}

func TestWireTaskPayloadAndStage(t *testing.T) {
	payload := conversion.ExecutionOptions{DeleteMode: "soft"}
	wirecheck.AssertSchema(t, "conversion_ExecutionOptions", payload)
	if wirecheck.Validate("conversion_ExecutionOptions", []byte(`{"renamed":"soft"}`)) == nil {
		t.Fatal("accepted renamed task field")
	}
	for _, tc := range []struct {
		stage  string
		result any
	}{
		{"skipped", refresh.Outcome{Action: "skip"}}, {"metadata", refresh.WorkResult{}},
	} {
		raw, err := json.Marshal(tc.result)
		if err != nil {
			t.Fatal(err)
		}
		component := executionComponentResponse{
			Status: "succeeded", Stage: tc.stage, TaskResult: raw, KeptDecisions: json.RawMessage(`[]`),
			Committed: []string{}, Removed: []string{}, Remaining: []string{}, Recovery: []string{},
		}
		wirecheck.AssertSchema(t, "metadata_refresh_v1_executionComponentResponse", component)
		if tc.stage == "skipped" {
			component.Stage = "metadata"
		} else {
			component.Stage = "skipped"
		}
		if wirecheck.Validate("metadata_refresh_v1_executionComponentResponse", component) == nil {
			t.Error("accepted the other metadata stage's result")
		}
	}
}

// The session_snapshot event and the planning-session detail share one schema,
// and that schema requires operation_type.
func TestWireSnapshotRequiresOperationType(t *testing.T) {
	wirecheck.AssertSchema(
		t,
		"generationViewResponse",
		generationViewResponse{OperationType: "conversion", Status: "queued"},
	)
	var withoutOperation map[string]any
	raw, err := json.Marshal(generationViewResponse{Status: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &withoutOperation); err != nil {
		t.Fatal(err)
	}
	delete(withoutOperation, "operation_type")
	if wirecheck.Validate("generationViewResponse", withoutOperation) == nil {
		t.Fatal("a snapshot without operation_type was accepted")
	}
}

// Closed value sets are part of the schema: a value the backend does not list is
// contract drift, whether it comes from a tag, a domain WireValues list or the
// registered Task kinds.
func TestWireRejectsValuesOutsideEnum(t *testing.T) {
	ok := generationViewResponse{OperationType: "conversion", Status: "queued"}
	wirecheck.AssertSchema(t, "generationViewResponse", ok)
	for name, value := range map[string]generationViewResponse{
		"status":         {OperationType: "conversion", Status: "paused"},
		"operation_type": {OperationType: "compress", Status: "queued"},
	} {
		if wirecheck.Validate("generationViewResponse", value) == nil {
			t.Errorf("accepted an unlisted %s", name)
		}
	}
	run := cleanupRunResponse{Status: "running", Categories: []cleanupCategory{"works"}, Phase: "works"}
	wirecheck.AssertSchema(t, "cleanupRunResponse", run)
	run.Categories = []cleanupCategory{"thumbnails"}
	if wirecheck.Validate("cleanupRunResponse", run) == nil {
		t.Error("accepted an unlisted cleanup category")
	}
}
