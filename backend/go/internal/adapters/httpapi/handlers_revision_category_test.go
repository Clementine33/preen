package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
)

func TestRevisionCategoryQueryHTTP(t *testing.T) {
	get, wsID, _ := httpapi.MemberReviewHTTPFixture(t)
	base := "/api/v1/worksets/" + wsID + "/operations/conversion/revisions/member-plan"
	for _, query := range []string{"?limit=5", "?category=x", "?category=x&limit=0", "?category=x&limit=-1",
		"?category=x&limit=abc", "?category=&limit=5"} {
		if response := get(base + query); response.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", query, response.Code, response.Body.String())
		}
	}
	response := get(base + "?category=x&limit=5")
	var body struct {
		CategoryCounts map[string]int    `json:"category_counts"`
		CategoryTotal  *int              `json:"category_total"`
		Members        []json.RawMessage `json:"members"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil ||
		body.CategoryCounts == nil || body.CategoryTotal == nil || *body.CategoryTotal != 0 || len(body.Members) != 0 {
		t.Fatalf("category read = %d %s", response.Code, response.Body.String())
	}
	var plain map[string]json.RawMessage
	if response = get(base); json.Unmarshal(response.Body.Bytes(), &plain) != nil {
		t.Fatal(response.Body.String())
	}
	if _, exists := plain["category_total"]; exists {
		t.Fatal("unfiltered read carries category_total")
	}
}
