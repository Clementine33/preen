package httpapi_test

import (
	"net/http"
	"testing"
)

func TestCollectionsPaging(t *testing.T) {
	server, repo := collectionServer(t)
	seedCollectionWorks(t, repo, "RJ00000001", "RJ00000002", "RJ00000003")
	path := newCollectionPath(t, server, "Page")
	collectionRequest(t, server, http.MethodPost, path+"/works",
		map[string]any{"rjs": []string{"RJ00000003", "RJ00000001", "RJ00000002"}}, 200)
	page := collectionRequest(t, server, http.MethodGet, path+"?limit=1&page=2", nil, 200)
	if page["total"] != float64(3) || page["work_count"] != float64(3) ||
		collectionValue[map[string]any](t, collectionValue[[]any](t, page["works"])[0])["rj"] != "RJ00000001" {
		t.Fatalf("detail page = %v", page)
	}
	for _, base := range []string{"/api/v1/collections", path} {
		for _, query := range []string{"limit=0", "limit=201", "limit=-1", "limit=1.5", "limit=abc", "page=0", "page=-1", "page=1.5", "page=abc"} {
			result := collectionRequest(t, server, http.MethodGet, base+"?"+query, nil, 400)
			if result["code"] != "INVALID_PAGE" {
				t.Fatalf("paging refusal = %v", result)
			}
		}
		collectionRequest(t, server, http.MethodGet, base+"?unknown=1", nil, 400)
	}
	page = collectionRequest(t, server, http.MethodGet, "/api/v1/collections?limit=1&page=2", nil, 200)
	if page["total"] != float64(2) || len(collectionValue[[]any](t, page["collections"])) != 1 ||
		collectionValue[map[string]any](t, collectionValue[[]any](t, page["collections"])[0])["name"] != "Page" {
		t.Fatalf("list page = %v", page)
	}
	for _, query := range []string{"page=99", "limit=200&page=9223372036854775807"} {
		page = collectionRequest(t, server, http.MethodGet, path+"?"+query, nil, 200)
		if page["total"] != float64(3) || len(collectionValue[[]any](t, page["works"])) != 0 {
			t.Fatalf("past end = %v", page)
		}
	}
}

func TestCollectionsRejectInvalidBatchesAtomically(t *testing.T) {
	server, repo := collectionServer(t)
	seedCollectionWorks(t, repo, "RJ00000001")
	if _, err := repo.DB().
		Exec("INSERT INTO works(id,created_at,updated_at) VALUES('RJ00000002','now','now')"); err != nil {
		t.Fatal(err)
	}
	path := newCollectionPath(t, server, "Validation")
	for _, test := range []struct {
		rjs    []string
		status int
		code   string
		bad    int
	}{
		{[]string{}, 400, "INVALID_ARGUMENT", 0},
		{[]string{"RJ00000001", "invalid"}, 400, "INVALID_ARGUMENT", 0},
		{[]string{"RJ00000001", "RJ00000002"}, 409, "WORK_IS_PLACEHOLDER", 1},
		{[]string{"RJ00000001", "RJ00000003", "RJ00000004"}, 404, "WORK_NOT_FOUND", 2},
		{[]string{"RJ00000002", "RJ00000003"}, 404, "WORK_NOT_FOUND", 2},
		{make([]string, 201), 400, "INVALID_ARGUMENT", 0},
	} {
		result := collectionRequest(
			t,
			server,
			http.MethodPost,
			path+"/works",
			map[string]any{"rjs": test.rjs},
			test.status,
		)
		if result["code"] != test.code ||
			test.bad > 0 && len(collectionValue[[]any](t, result["details"])) != test.bad {
			t.Fatalf("batch refusal = %v", result)
		}
		assertCollectionWorks(t, server, path)
	}
	collectionRequest(
		t,
		server,
		http.MethodPatch,
		"/api/v1/works/RJ00000002/user-data",
		map[string]any{"favorite": true},
		409,
	)
	for _, body := range []any{nil, map[string]any{}, map[string]any{"name": " \t "}, map[string]any{"name": "ok", "extra": true}} {
		collectionRequest(t, server, http.MethodPost, "/api/v1/collections", body, 400)
	}
	collectionRequest(t, server, http.MethodPatch, path, map[string]any{"name": " "}, 400)
	for _, body := range []any{nil, map[string]any{}, map[string]any{"before_rj": 3}, map[string]any{"before_rj": "invalid"}} {
		collectionRequest(t, server, http.MethodPatch, path+"/works/RJ00000001", body, 400)
	}
	for _, method := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
		collectionRequest(t, server, method, "/api/v1/collections/missing", map[string]any{"name": "x"}, 404)
	}
	collectionRequest(
		t,
		server,
		http.MethodPost,
		"/api/v1/collections/missing/works",
		map[string]any{"rjs": []string{"RJ00000001"}},
		404,
	)
	collectionRequest(t, server, http.MethodDelete, "/api/v1/collections/missing/works/RJ00000001", nil, 404)
}
