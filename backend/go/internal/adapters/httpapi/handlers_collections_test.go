package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/adapters/httpapi"
	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/collections"
	"github.com/onsei/organizer/backend/internal/works"
)

func TestCollectionsCreateAndList(t *testing.T) {
	server, _ := collectionServer(t)
	created := collectionRequest(
		t,
		server,
		http.MethodPost,
		"/api/v1/collections",
		map[string]any{"name": "  Sleep  "},
		201,
	)
	if created["name"] != "Sleep" || created["work_count"] != float64(0) || created["id"] == "" {
		t.Fatalf("created collection = %v", created)
	}
	page := collectionRequest(t, server, http.MethodGet, "/api/v1/collections", nil, 200)
	rows := collectionValue[[]any](t, page["collections"])
	if page["total"] != float64(2) || collectionValue[map[string]any](t, rows[0])["id"] != "favorites" ||
		collectionValue[map[string]any](t, rows[1])["id"] != created["id"] {
		t.Fatalf("collection list = %v", page)
	}
}

func TestCollectionsAppendPreservesOrderAndDeduplicates(t *testing.T) {
	server, repo := collectionServer(t)
	seedCollectionWorks(t, repo, "RJ00000001", "RJ00000002", "RJ00000003")
	path := newCollectionPath(t, server, "Sleep")
	collectionRequest(t, server, http.MethodPost, path+"/works",
		map[string]any{"rjs": []string{"rj000002", "RJ00000001", "RJ00000002"}}, 200)
	result := collectionRequest(t, server, http.MethodPost, path+"/works",
		map[string]any{"rjs": []string{"RJ00000001", "RJ00000003"}}, 200)
	if result["work_count"] != float64(3) || len(collectionValue[[]any](t, result["added"])) != 1 ||
		len(collectionValue[[]any](t, result["skipped"])) != 1 {
		t.Fatalf("append result = %v", result)
	}
	assertCollectionWorks(t, server, path, "RJ00000002", "RJ00000001", "RJ00000003")
}

func TestCollectionsMoveAppliesToCurrentMembership(t *testing.T) {
	server, repo := collectionServer(t)
	seedCollectionWorks(t, repo, "RJ00000001", "RJ00000002", "RJ00000003", "RJ00000004")
	path := newCollectionPath(t, server, "Move")
	collectionRequest(t, server, http.MethodPost, path+"/works",
		map[string]any{"rjs": []string{"RJ00000001", "RJ00000002", "RJ00000003"}}, 200)
	collectionRequest(t, server, http.MethodPost, path+"/works", map[string]any{"rjs": []string{"RJ00000004"}}, 200)
	collectionRequest(
		t,
		server,
		http.MethodPatch,
		path+"/works/RJ00000003",
		map[string]any{"before_rj": "rj000001"},
		204,
	)
	assertCollectionWorks(t, server, path, "RJ00000003", "RJ00000001", "RJ00000002", "RJ00000004")
	collectionRequest(t, server, http.MethodPatch, path+"/works/RJ00000003", map[string]any{"before_rj": nil}, 204)
	assertCollectionWorks(t, server, path, "RJ00000001", "RJ00000002", "RJ00000004", "RJ00000003")
	collectionRequest(t, server, http.MethodDelete, path+"/works/RJ00000002", nil, 204)
	collectionRequest(t, server, http.MethodDelete, path+"/works/RJ00000002", nil, 204)
	for _, move := range []struct{ work, before string }{
		{"RJ00000001", "RJ00000002"}, {"RJ00000002", "RJ00000001"},
	} {
		result := collectionRequest(t, server, http.MethodPatch, path+"/works/"+move.work,
			map[string]any{"before_rj": move.before}, 409)
		if result["code"] != "COLLECTION_MEMBER_CONFLICT" {
			t.Fatalf("conflict = %v", result)
		}
	}
	assertCollectionWorks(t, server, path, "RJ00000001", "RJ00000004", "RJ00000003")
	collectionRequest(t, server, http.MethodPatch, path+"/works/RJ00000001", map[string]any{}, 400)
	collectionRequest(
		t,
		server,
		http.MethodPatch,
		path+"/works/RJ00000001",
		map[string]any{"before_rj": "RJ00000001"},
		204,
	)
	collectionRequest(
		t,
		server,
		http.MethodPatch,
		path+"/works/RJ00000001",
		map[string]any{"before_rj": "RJ00000004"},
		204,
	)
	assertCollectionWorks(t, server, path, "RJ00000001", "RJ00000004", "RJ00000003")
}

func TestCollectionsFavoritesAreTheFavoriteFlag(t *testing.T) {
	server, repo := collectionServer(t)
	seedCollectionWorks(t, repo, "RJ00000001", "RJ00000002")
	favorites := "/api/v1/collections/favorites"
	collectionRequest(t, server, http.MethodPatch, "/api/v1/works/RJ00000002/user-data",
		map[string]any{"favorite": true, "rating": 8, "note": "keep"}, 200)
	assertCollectionWorks(t, server, favorites, "RJ00000002")
	collectionRequest(
		t,
		server,
		http.MethodPost,
		favorites+"/works",
		map[string]any{"rjs": []string{"RJ00000001"}},
		200,
	)
	collectionRequest(
		t,
		server,
		http.MethodPatch,
		"/api/v1/works/RJ00000002/user-data",
		map[string]any{"favorite": true},
		200,
	)
	assertCollectionWorks(t, server, favorites, "RJ00000002", "RJ00000001")
	detail := collectionRequest(t, server, http.MethodGet, "/api/v1/works/RJ00000001", nil, 200)
	if detail["favorite"] != true {
		t.Fatalf("work favorite = %v", detail)
	}
	filtered := collectionRequest(t, server, http.MethodGet, "/api/v1/works?favorite=true", nil, 200)
	if filtered["total"] != float64(2) {
		t.Fatalf("favorite filter = %v", filtered)
	}
	collectionRequest(t, server, http.MethodDelete, favorites+"/works/RJ00000002", nil, 204)
	detail = collectionRequest(t, server, http.MethodGet, "/api/v1/works/RJ00000002", nil, 200)
	if detail["favorite"] != false || detail["rating"] != float64(8) || detail["note"] != "keep" {
		t.Fatalf("personal data = %v", detail)
	}
	collectionRequest(
		t,
		server,
		http.MethodPatch,
		"/api/v1/works/RJ00000001/user-data",
		map[string]any{"favorite": false},
		200,
	)
	assertCollectionWorks(t, server, favorites)
}

func TestCollectionsDeletionProtectsWorksAndBuiltin(t *testing.T) {
	server, repo := collectionServer(t)
	seedCollectionWorks(t, repo, "RJ00000001")
	path := newCollectionPath(t, server, "Sleep")
	other := newCollectionPath(t, server, "Sleep")
	collectionRequest(t, server, http.MethodPatch, path, map[string]any{"name": "  Rest  "}, 200)
	for _, target := range []string{path, other} {
		collectionRequest(
			t,
			server,
			http.MethodPost,
			target+"/works",
			map[string]any{"rjs": []string{"RJ00000001"}},
			200,
		)
	}
	blocked := collectionRequest(t, server, http.MethodDelete, "/api/v1/works/RJ00000001", nil, 409)
	if blocked["code"] != "WORK_IN_USE" || len(collectionValue[[]any](t, blocked["details"])) != 2 {
		t.Fatalf("deletion refusal = %v", blocked)
	}
	collectionRequest(t, server, http.MethodDelete, path, nil, 204)
	assertCollectionWorks(t, server, other, "RJ00000001")
	collectionRequest(t, server, http.MethodGet, path, nil, 404)
	collectionRequest(t, server, http.MethodDelete, other+"/works/RJ00000001", nil, 204)
	collectionRequest(
		t,
		server,
		http.MethodPatch,
		"/api/v1/works/RJ00000001/user-data",
		map[string]any{"favorite": true},
		200,
	)
	collectionRequest(t, server, http.MethodDelete, "/api/v1/works/RJ00000001", nil, 409)
	collectionRequest(t, server, http.MethodDelete, "/api/v1/collections/favorites/works/RJ00000001", nil, 204)
	collectionRequest(t, server, http.MethodDelete, "/api/v1/works/RJ00000001", nil, 204)
	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		result := collectionRequest(
			t,
			server,
			method,
			"/api/v1/collections/favorites",
			map[string]any{"name": "Other"},
			409,
		)
		if result["code"] != "BUILTIN_COLLECTION" {
			t.Fatalf("builtin refusal = %v", result)
		}
	}
}

func seedCollectionWorks(t *testing.T, repo *sqlite.Repository, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if _, err := repo.DB().Exec(`INSERT INTO works(id,title,collected_at,created_at,updated_at)
		 VALUES(?,?,'2026-10-14T00:00:00.000000000Z','now','now')`, id, id); err != nil {
			t.Fatal(err)
		}
	}
}

func newCollectionPath(t *testing.T, server http.Handler, name string) string {
	t.Helper()
	result := collectionRequest(t, server, http.MethodPost, "/api/v1/collections", map[string]any{"name": name}, 201)
	return "/api/v1/collections/" + collectionValue[string](t, result["id"])
}

func assertCollectionWorks(t *testing.T, server http.Handler, path string, want ...string) {
	t.Helper()
	result := collectionRequest(t, server, http.MethodGet, path, nil, 200)
	rows := collectionValue[[]any](t, result["works"])
	if len(rows) != len(want) {
		t.Fatalf("works = %v, want %v", rows, want)
	}
	for i, id := range want {
		if collectionValue[map[string]any](t, rows[i])["rj"] != id {
			t.Fatalf("works = %v, want %v", rows, want)
		}
	}
}

func collectionServer(t *testing.T) (http.Handler, *sqlite.Repository) {
	t.Helper()
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return collectionHandler(repo), repo
}

func collectionHandler(repo *sqlite.Repository) http.Handler {
	return httpapi.NewServer(httpapi.Dependencies{Collections: collections.NewService(repo),
		Works:       works.NewService(repo, repo, filesystem.ResolveMember, filesystem.ResolveMemberFile, "test"),
		LocalCovers: repo})
}

func collectionRequest(t *testing.T, server http.Handler, method, path string, body any, status int) map[string]any {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, response.Code, status, response.Body.String())
	}
	var result map[string]any
	if status != http.StatusNoContent {
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func collectionValue[T any](t *testing.T, value any) T {
	t.Helper()
	typed, ok := value.(T)
	if !ok {
		t.Fatalf("unexpected response value %T: %v", value, value)
	}
	return typed
}
