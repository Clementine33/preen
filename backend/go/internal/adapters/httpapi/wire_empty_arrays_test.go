//nolint:testpackage // exercises the private DTO conversions with empty domain values
package httpapi

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Clementine33/preen/backend/internal/cleanup"
	"github.com/Clementine33/preen/backend/internal/collections"
	"github.com/Clementine33/preen/backend/internal/testutil/wirecheck"
	"github.com/Clementine33/preen/backend/internal/works"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
	"github.com/Clementine33/preen/backend/internal/workset"
)

// Response arrays are never null: an empty domain collection converts to [].
func TestWireEmptyCollectionsEncodeAsEmptyArrays(t *testing.T) {
	detail := toWorkDetail(&works.WorkDetail{}, func(works.Copy) string { return "" })
	envelope := toTaskEnvelope(workset.RevisionPlan{})
	view := toExecutionViewResponse(&workset.ExecutionView{OperationType: "conversion", Status: "queued"})
	members := toExecutionViewResponse(&workset.ExecutionView{
		OperationType: "conversion", Status: "queued", Members: &workset.ExecutionMembers{},
	})
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"workDetailResponse", detail, `"edition_tree":[]`},
		{"workDetailResponse", detail, `"language_editions":[]`},
		{"collectionWorksAppendResponse", toCollectionAppendResponse(collections.AppendResult{}), `"added":[]`},
		{"coverReportResponse", toCoverReportResponse(coverdelivery.Report{}), `"items":[]`},
		{"cleanupPreviewResponse", toCleanupPreviewResponse(cleanup.Preview{}), `"categories":[]`},
		{"preferencesResponse", toPreferencesResponse(works.Preferences{}), `"track_formats":[]`},
		{"taskClassifierSnapshot", envelope.Payload.Classifier, `"tags":[]`},
		{"conversion_v1_executionFullViewResponse", view, `"components":[]`},
		{"conversion_v1_executionMembersViewResponse", members, `"attention":[]`},
		{"conversion_v1_executionComponentResponse", toExecutionComponentResponse(workset.ExecutionComponentView{Status: "pending", KeptDecisions: json.RawMessage(`[]`)}), `"committed":[]`},
	} {
		t.Run(tc.name+tc.want, func(t *testing.T) {
			wirecheck.AssertSchema(t, tc.name, tc.value)
			raw, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(raw, []byte(tc.want)) {
				t.Fatalf("%s missing %s", raw, tc.want)
			}
		})
	}
}
