package httpapi //nolint:testpackage // exercises HTTP responses using the shared server fixtures and wire DTOs

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/works"
)

func TestWorkDetailExternalBridgeDeletionAndRescan(t *testing.T) {
	var repo *sqlite.Repository
	handler := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		repo = fixture
		wireWorks(d, fixture)
		seedHTTPWork(t, repo, "RJ00000001", "translation")
		seedHTTPWork(t, repo, "RJ01289610", "page")
		catalog := works.NewService(repo, nil, nil, nil, "")
		for id, report := range map[works.WorkCode]works.ImportRelations{
			"RJ00000001": {Role: "child", ParentRJ: new("RJ01289610")},
			"RJ01289610": {Role: "parent", Language: new("CHI_HANS"), ParentRJ: new("RJ01287230"), OriginalRJ: new("RJ01287230")},
		} {
			report.Source, report.FetchedAt = "dlsite", stamp
			if _, err := catalog.ImportRelationReport(id, report); err != nil {
				t.Fatal(err)
			}
		}
		// Fetched URL text is never used for the lightweight navigation links.
		if _, err := repo.DB().
			Exec(`UPDATE works SET source_url='javascript:unsafe' WHERE id='RJ00000001'`); err != nil {
			t.Fatal(err)
		}
	})

	assertBridgePage(t, handler, true, false)
	deleted := doRequest(t, handler, http.MethodDelete, "/api/v1/works/RJ01289610", nil, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", deleted.Code, deleted.Body.String())
	}
	assertBridgePage(t, handler, false, false)
	external := doRequest(t, handler, http.MethodGet, "/api/v1/works/RJ01289610", nil, nil)
	if external.Code != http.StatusNotFound {
		t.Fatalf("external full detail: %d %s", external.Code, external.Body.String())
	}
	lib, err := repo.CreateLibrary("rescan", "/rescan")
	if err != nil {
		t.Fatal(err)
	}
	staging := sqlite.NewScanStaging(repo)
	if err := staging.WriteStagingEntries("rescan", []inventory.StagingEntry{
		{
			SessionID:  "rescan",
			RootPath:   "/rescan",
			Path:       "/rescan/RJ01289610",
			ParentPath: "/rescan",
			Name:       "RJ01289610",
			IsDir:      true,
			IsWorkRoot: true,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := staging.MergeStaging("rescan", lib.ID, "/rescan"); err != nil {
		t.Fatal(err)
	}
	assertBridgePage(t, handler, true, true)
	local := doRequest(t, handler, http.MethodGet, "/api/v1/works/RJ01289610", nil, nil)
	if local.Code != http.StatusOK {
		t.Fatalf("rescanned full detail: %d %s", local.Code, local.Body.String())
	}
}

func TestWorkDetailReportsRejectedClaimsWithoutDrawingThem(t *testing.T) {
	handler := newTestServer(t, func(d *Dependencies, repo *sqlite.Repository) {
		wireWorks(d, repo)
		seedHTTPWork(t, repo, "RJ00000001", "page")
		catalog := works.NewService(repo, nil, nil, nil, "")
		for id, report := range map[works.WorkCode]works.ImportRelations{
			"RJ00000001": {Role: "parent", Language: new("CHI_HANS"), ChildRJs: []string{"RJ00000002", "RJ00000003"}},
			"RJ00000002": {Role: "child", ParentRJ: new("RJ00000004")},
			"RJ00000004": {Role: "parent", ChildRJs: []string{"RJ00000003"}},
			"RJ00000005": {Role: "parent", ChildRJs: []string{"RJ00000006"}},
			"RJ00000007": {Role: "parent", ChildRJs: []string{"RJ00000006"}},
		} {
			report.Source, report.FetchedAt = "dlsite", stamp
			if _, err := catalog.ImportRelationReport(id, report); err != nil {
				t.Fatal(err)
			}
		}
	})
	response := doRequest(t, handler, http.MethodGet, "/api/v1/works/RJ00000001", nil, nil)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var detail struct {
		Tree     []editionSlotResponse   `json:"edition_tree"`
		Warnings []works.RelationWarning `json:"relation_warnings"`
		Nodes    []treeNodeResponse      `json:"relation_nodes"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Tree) != 0 || len(detail.Nodes) != 1 || len(detail.Warnings) != 2 ||
		detail.Warnings[0].Code != works.CodeRelationConflict ||
		detail.Warnings[0].WorkID != "RJ00000002" ||
		detail.Warnings[1].Code != works.CodeRelationAmbiguous ||
		detail.Warnings[1].WorkID != "RJ00000003" {
		t.Fatalf("rejected claims: %+v", detail)
	}
}

func assertBridgePage(t *testing.T, handler http.Handler, collected, held bool) {
	t.Helper()
	response := doRequest(t, handler, http.MethodGet, "/api/v1/works/RJ00000001", nil, nil)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var detail struct {
		Relation relationResponse        `json:"relation"`
		Nodes    []treeNodeResponse      `json:"relation_nodes"`
		Tree     []editionSlotResponse   `json:"edition_tree"`
		Warnings []works.RelationWarning `json:"relation_warnings"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Relation.ParentRJ == nil || *detail.Relation.ParentRJ != "RJ01289610" ||
		detail.Relation.OriginalRJ == nil ||
		*detail.Relation.OriginalRJ != "RJ01287230" ||
		len(detail.Nodes) != 3 ||
		len(detail.Warnings) != 0 {
		t.Fatalf("bridge: %+v", detail)
	}
	page := detail.Nodes[2]
	if page.RJ != "RJ01289610" || page.IsWork != collected || page.Held != held || page.Language == nil ||
		*page.Language != "CHI_HANS" ||
		*page.Relation.ParentRJ != "RJ01287230" ||
		page.SourceURL == nil ||
		*page.SourceURL != "https://www.dlsite.com/maniax/work/=/product_id/RJ01289610.html" {
		t.Fatalf("page: %+v", page)
	}
	if detail.Nodes[1].Language != nil || detail.Nodes[1].Relation.Role != nil {
		t.Fatalf("unknown original: %+v", detail.Nodes[1])
	}
	if *detail.Nodes[0].SourceURL != "https://www.dlsite.com/maniax/work/=/product_id/RJ000001.html" {
		t.Fatalf("unsafe source link: %+v", detail.Nodes[0])
	}
	if len(detail.Tree) != 2 || detail.Tree[1].Root.IsWork != collected || detail.Tree[1].Root.Held != held {
		t.Fatalf("tree: %+v", detail.Tree)
	}
}
