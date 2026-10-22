package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/collections"
	"github.com/Clementine33/preen/backend/internal/works"
)

func deletionServer(t *testing.T) (http.Handler, *sqlite.Repository) {
	t.Helper()
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if _, err := repo.DB().Exec(`INSERT INTO works(id,collected_at,created_at,updated_at)
	 VALUES('RJ00000001','2026-09-27T10:00:00.000000000Z','','')`); err != nil {
		t.Fatal(err)
	}
	return httpapi.NewServer(httpapi.Dependencies{
		Works:       works.NewService(repo, nil, nil, nil, "test"),
		Collections: collections.NewService(repo),
	}), repo
}

func deletionRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func TestWorkDeletionReportsEveryPersonalDataBlocker(t *testing.T) {
	handler, repo := deletionServer(t)
	for _, query := range []string{
		`INSERT INTO work_user_data(work_id,rating,note) VALUES('RJ00000001',8,' ')`,
		`INSERT INTO user_tags VALUES('tag','Keep')`,
		`INSERT INTO work_user_tags VALUES('RJ00000001','tag')`,
		`INSERT INTO collection_works VALUES('favorites','RJ00000001',0)`,
		`INSERT INTO collections VALUES('custom','Sleep','','')`,
		`INSERT INTO collection_works VALUES('custom','RJ00000001',0)`,
		`INSERT INTO work_listening VALUES('RJ00000001','',NULL,NULL,NULL,NULL,'finished','client',1,'')`,
		`INSERT INTO libraries(id,name,root_path,created_at,updated_at) VALUES('offline','Offline','/nonexistent-preen-library','','')`,
		`INSERT INTO work_copies VALUES('copy','RJ00000001','offline','work','')`,
	} {
		if _, err := repo.DB().Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	response := deletionRequest(handler, http.MethodDelete, "works/RJ00000001", "")
	var envelope struct {
		Code    string   `json:"code"`
		Details []string `json:"details"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusConflict || envelope.Code != works.CodeWorkInUse || len(envelope.Details) != 7 {
		t.Fatalf("delete=%d %s", response.Code, response.Body)
	}
	for _, reason := range []string{
		"a personal rating is saved for this work",
		"a personal note is saved for this work",
		"user tags are attached to this work",
		"listening records are saved for this work",
		"a local copy is bound to this work",
		"a collection holds this work: custom Sleep",
		"a collection holds this work: favorites Favorites",
	} {
		if !slices.Contains(envelope.Details, reason) {
			t.Errorf("details=%v, missing %q", envelope.Details, reason)
		}
	}
}

func TestWorkDeletionMakesHTTPPersonalEditsReturnNotFound(t *testing.T) {
	handler, _ := deletionServer(t)
	if response := deletionRequest(
		handler,
		http.MethodDelete,
		"works/RJ00000001",
		"",
	); response.Code != http.StatusNoContent {
		t.Fatalf("delete=%d %s", response.Code, response.Body)
	}
	for _, request := range []struct{ method, path, body string }{
		{http.MethodDelete, "works/RJ00000001", ""},
		{http.MethodPatch, "works/RJ00000001/user-data", `{"rating":8,"favorite":true,"note":"keep"}`},
		{http.MethodPut, "works/RJ00000001/user-tags", `{"tag_ids":[]}`},
		{http.MethodPut, "works/RJ00000001/listening", `{"track_key":"track","track_count":1,"client":"client","seq":1}`},
		{http.MethodDelete, "works/RJ00000001/listening", ""},
		{http.MethodPost, "collections/favorites/works", `{"rjs":["RJ00000001"]}`},
		{http.MethodDelete, "collections/favorites/works/RJ00000001", ""},
		{http.MethodPatch, "collections/favorites/works/RJ00000001", `{"before_rj":null}`},
	} {
		t.Run(request.method+" "+request.path, func(t *testing.T) {
			response := deletionRequest(handler, request.method, request.path, request.body)
			var envelope struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusNotFound || envelope.Code != works.CodeWorkNotFound {
				t.Fatalf("edit=%d %s, want WORK_NOT_FOUND", response.Code, response.Body)
			}
		})
	}
}
