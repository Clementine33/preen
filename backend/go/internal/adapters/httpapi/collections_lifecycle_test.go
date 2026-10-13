package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/adapters/httpapi"
	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/collections"
	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/library"
)

func TestCollectionsPersistAcrossRepositoryReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	repo, err := sqlite.NewRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	seedCollectionWorks(t, repo, "RJ00000001", "RJ00000002")
	server := collectionHandler(repo)
	collection := newCollectionPath(t, server, "Saved")
	collectionRequest(
		t,
		server,
		http.MethodPost,
		collection+"/works",
		map[string]any{"rjs": []string{"RJ00000002", "RJ00000001"}},
		200,
	)
	if closeErr := repo.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	reopened, err := sqlite.NewRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	assertCollectionWorks(t, collectionHandler(reopened), collection, "RJ00000002", "RJ00000001")
}

func TestCollectionsSurviveScanAndLibraryRemoval(t *testing.T) {
	_, repo := collectionServer(t)
	server := httpapi.NewServer(httpapi.Dependencies{
		Collections: collections.NewService(repo),
		Library:     library.NewService(repo, nil, repo, filesystem.ResolveMember),
		Inventory: inventory.NewService(inventory.NewPipeline(
			sqlite.NewScanStaging(repo),
			filesystem.WalkRootEntriesParallel,
			filesystem.WalkFolderEntries,
			nil,
		), repo, nil, repo.UpdateLibraryScanState),
	})
	collection := newCollectionPath(t, server, "Across libraries")
	for _, rj := range []string{"RJ00000001", "RJ00000002"} {
		root := t.TempDir()
		member := filepath.Join(root, rj)
		if err := os.Mkdir(member, 0755); err != nil {
			t.Fatal(err)
		}
		lib := collectionRequest(t, server, http.MethodPost, "/api/v1/libraries",
			map[string]any{"name": rj, "root_path": root}, 201)
		libPath := "/api/v1/libraries/" + collectionValue[string](t, lib["id"])
		scanCollectionLibrary(t, server, libPath)
		collectionRequest(t, server, http.MethodPost, collection+"/works", map[string]any{"rjs": []string{rj}}, 200)
		if err := os.Remove(member); err != nil {
			t.Fatal(err)
		}
		scanCollectionLibrary(t, server, libPath)
		collectionRequest(t, server, http.MethodDelete, libPath, nil, 204)
	}
	assertCollectionWorks(t, server, collection, "RJ00000001", "RJ00000002")
	page := collectionRequest(t, server, http.MethodGet, collection, nil, 200)
	for _, row := range collectionValue[[]any](t, page["works"]) {
		if collectionValue[map[string]any](t, row)["copy_count"] != float64(0) {
			t.Fatalf("lost-copy summary = %v", row)
		}
	}
	collectionRequest(
		t,
		server,
		http.MethodPatch,
		collection+"/works/RJ00000002",
		map[string]any{"before_rj": "RJ00000001"},
		204,
	)
	assertCollectionWorks(t, server, collection, "RJ00000002", "RJ00000001")
}

func scanCollectionLibrary(t *testing.T, server http.Handler, path string) {
	t.Helper()
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path+"/scans", strings.NewReader("{}")))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "event: completed") {
		t.Fatalf("scan = %d %s", response.Code, response.Body.String())
	}
}
