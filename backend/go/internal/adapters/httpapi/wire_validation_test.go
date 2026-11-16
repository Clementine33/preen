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
		{"reconcile_ComponentOutcome", reconcile.ComponentOutcome{}},
		{"reconcile_VariantDecision", reconcile.VariantDecision{}},
		{"workContentNodeResponse", workContentNodeResponse{Name: "directory"}},
		{"workContentNodeResponse", workContentNodeResponse{Name: "file", Kind: "audio"}},
		{"conversion_v1_executionViewResponse", executionViewResponse{OperationType: "conversion"}},
		{"conversion_v1_executionViewResponse", executionViewResponse{OperationType: "conversion", Members: &executionMembersResponse{}}},
		{"conversion_v1_executionViewResponse", executionViewResponse{OperationType: "conversion", Member: &executionMemberSummaryResponse{}}},
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
		{"skipped", refresh.Outcome{}}, {"metadata", refresh.WorkResult{}},
	} {
		raw, err := json.Marshal(tc.result)
		if err != nil {
			t.Fatal(err)
		}
		component := executionComponentResponse{Stage: tc.stage, TaskResult: raw, KeptDecisions: json.RawMessage(`[]`)}
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

func TestWireSnapshotIsDistinctFromHTTPDetail(t *testing.T) {
	wirecheck.AssertSchema(t, "generationSnapshotEvent", generationSnapshotEvent{})
	if wirecheck.Validate("generationViewResponse", generationSnapshotEvent{}) == nil {
		t.Fatal("snapshot accepted as HTTP detail")
	}
}

func TestWireLegacySnapshotsHaveIndependentSchemas(t *testing.T) {
	legacy := []byte(`{"schema_version":1,"classifier_tags":["legacy"],"matched":{},"unmatched":{}}`)
	wirecheck.AssertSchema(t, "conversion_DraftDoc_stored", legacy)
	wirecheck.AssertSchema(t, "reconcile_Policy_stored", legacy)
	if wirecheck.Validate("conversion_DraftDoc", legacy) == nil {
		t.Fatal("legacy storage accepted as current canonical encoding")
	}
	if wirecheck.Validate("draftDocumentRequest", legacy) == nil {
		t.Fatal("compatibility key accepted as a draft request")
	}
}
