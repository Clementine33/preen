//nolint:testpackage // reuses real HTTP repository fixtures for contract coverage
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestWireCatalogAndWorksetListings(t *testing.T) {
	server := newTestServer(t, func(d *Dependencies, repo *sqlite.Repository) { wireWorks(d, repo) })
	if w := doRequest(t, server, http.MethodGet, "/api/v1/user-tags", nil, nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w := doRequest(t, server, http.MethodPost, "/api/v1/user-tags", map[string]string{"name": "before"}, nil)
	var tag userTagResponse
	if err := json.Unmarshal(w.Body.Bytes(), &tag); err != nil {
		t.Fatal(err)
	}
	if w = doRequest(
		t,
		server,
		http.MethodPatch,
		"/api/v1/user-tags/"+tag.ID,
		map[string]string{"name": "after"},
		nil,
	); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	h, _ := newWorksetServer(t)
	if w = req(t, h, http.MethodGet, "/api/v1/worksets", testToken, nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}

type replayWireService struct{ workset.Service }

func (replayWireService) StartGeneration(
	context.Context,
	string,
	string,
	workset.StartGenerationRequest,
) (*workset.StartGenerationResult, error) {
	return &workset.StartGenerationResult{Revision: &workset.RevisionSummary{PlanID: "existing"}}, nil
}
func TestWireGenerationRevisionReplay(t *testing.T) {
	server := newTestServer(t, func(d *Dependencies, _ *sqlite.Repository) { d.WorksetService = replayWireService{} })
	w := doRequest(
		t,
		server,
		http.MethodPost,
		"/api/v1/worksets/w/operations/conversion/revisions",
		nil,
		map[string]string{"If-Match": "1", "Idempotency-Key": "replay"},
	)
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

type coverWireService struct{ workset.Service }

func (coverWireService) GetExecution(
	context.Context,
	string,
	string,
	string,
	workset.ExecutionRead,
) (*workset.ExecutionView, error) {
	return &workset.ExecutionView{OperationType: "metadata_refresh"}, nil
}

type unusedWireDownloader struct{ coverdelivery.Downloader }

func TestWireCoverReports(t *testing.T) {
	server := newTestServer(t, func(d *Dependencies, repo *sqlite.Repository) {
		var err error
		d.Covers, err = coverdelivery.New(
			coverdelivery.Dependencies{
				Store:      repo,
				Downloader: unusedWireDownloader{},
				Files:      filesystem.CoverFiles{},
				Acquire:    func() (func(), error) { return func() {}, nil },
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		d.WorksetService = coverWireService{}
	})
	base := "/api/v1/worksets/w/operations/metadata_refresh/executions/e/covers"
	for _, tc := range []struct{ method, path string }{{http.MethodGet, base}, {http.MethodPost, base + "/retry"}, {http.MethodPost, base + "/cancel"}} {
		w := doRequest(t, server, tc.method, tc.path, nil, nil)
		if w.Code != 200 {
			t.Fatalf("%s: status %d: %s", tc.path, w.Code, w.Body.String())
		}
	}
}
