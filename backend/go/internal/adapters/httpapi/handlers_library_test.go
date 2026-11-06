package httpapi //nolint:testpackage // white-box tests exercise unexported internals

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
)

//nolint:gocognit,funlen // CRUD scenario with many branches
func TestLibrariesCRUD(t *testing.T) {
	engine := newTestServer(t, nil) // empty token: no auth needed for CRUD flow

	// Set by the create subtest, read by the later ones (subtests run in order).
	var createdID string

	t.Run("create", func(t *testing.T) {
		w := doRequest(t, engine, http.MethodPost, "/api/v1/libraries",
			map[string]string{"name": "Music", "root_path": "/music"}, nil)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body=%s)", w.Code, w.Body.String())
		}
		var lib libraryResponse
		if err := json.Unmarshal(w.Body.Bytes(), &lib); err != nil {
			t.Fatalf("decode library: %v (body=%s)", err, w.Body.String())
		}
		if lib.ID == "" {
			t.Fatal("created library missing id")
		}
		if lib.Name != "Music" {
			t.Fatalf("name = %q, want Music", lib.Name)
		}
		if lib.RootPath != "/music" {
			t.Fatalf("root_path = %q, want /music", lib.RootPath)
		}
		createdID = lib.ID
	})

	t.Run("duplicate root path conflicts", func(t *testing.T) {
		w := doRequest(t, engine, http.MethodPost, "/api/v1/libraries",
			map[string]string{"name": "Music Two", "root_path": "/music"}, nil)
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409 (body=%s)", w.Code, w.Body.String())
		}
		code, _ := errorEnvelope(t, w)
		if code != "LIBRARY_EXISTS" {
			t.Fatalf("code = %q, want LIBRARY_EXISTS", code)
		}
	})

	t.Run("list", func(t *testing.T) {
		w := doRequest(t, engine, http.MethodGet, "/api/v1/libraries", nil, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
		}
		var out struct {
			Libraries []libraryResponse `json:"libraries"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode list: %v (body=%s)", err, w.Body.String())
		}
		if len(out.Libraries) != 1 {
			t.Fatalf("len(libraries) = %d, want 1", len(out.Libraries))
		}
		if out.Libraries[0].ID != createdID {
			t.Fatalf("list id = %q, want %q", out.Libraries[0].ID, createdID)
		}
	})

	t.Run("get by id", func(t *testing.T) {
		w := doRequest(t, engine, http.MethodGet, "/api/v1/libraries/"+createdID, nil, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
		}
		var lib libraryResponse
		if err := json.Unmarshal(w.Body.Bytes(), &lib); err != nil {
			t.Fatalf("decode library: %v", err)
		}
		if lib.Name != "Music" {
			t.Fatalf("name = %q, want Music", lib.Name)
		}
	})

	t.Run("patch name", func(t *testing.T) {
		w := doRequest(t, engine, http.MethodPatch, "/api/v1/libraries/"+createdID,
			map[string]string{"name": "Classical"}, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
		}
		var lib libraryResponse
		if err := json.Unmarshal(w.Body.Bytes(), &lib); err != nil {
			t.Fatalf("decode library: %v", err)
		}
		if lib.Name != "Classical" {
			t.Fatalf("name = %q, want Classical", lib.Name)
		}
		if lib.RootPath != "/music" {
			t.Fatalf("root_path = %q, want /music (unchanged by name-only patch)", lib.RootPath)
		}
	})

	t.Run("unknown id not found", func(t *testing.T) {
		w := doRequest(t, engine, http.MethodGet, "/api/v1/libraries/does-not-exist", nil, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body=%s)", w.Code, w.Body.String())
		}
		code, _ := errorEnvelope(t, w)
		if code != "LIBRARY_NOT_FOUND" {
			t.Fatalf("code = %q, want LIBRARY_NOT_FOUND", code)
		}
	})

	t.Run("delete", func(t *testing.T) {
		w := doRequest(t, engine, http.MethodDelete, "/api/v1/libraries/"+createdID, nil, nil)
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204 (body=%s)", w.Code, w.Body.String())
		}
		// Deleted: a subsequent get must 404.
		w = doRequest(t, engine, http.MethodGet, "/api/v1/libraries/"+createdID, nil, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("post-delete status = %d, want 404 (body=%s)", w.Code, w.Body.String())
		}
	})
}

func TestLibraryOverlapHTTPRefusals(t *testing.T) {
	engine := newTestServer(t, nil)
	createLibraryViaAPI(t, engine, "outer", "/a")
	createLibraryViaAPI(t, engine, "sibling", "/b")
	w := doRequest(t, engine, http.MethodPost, "/api/v1/libraries",
		map[string]string{"name": "nested", "root_path": "/a/nested"}, nil)
	code, message := errorEnvelope(t, w)
	if w.Code != http.StatusConflict || code != "LIBRARY_ROOT_OVERLAP" || message == "" {
		t.Fatalf("refusal: %d %s", w.Code, w.Body.String())
	}
}

// TestLibraryRootIsCheckedAtCreation: a root must exist as a directory (400
// LIBRARY_ROOT_INVALID), and it is fixed afterwards: PATCH has no root_path.
func TestLibraryRootIsCheckedAtCreation(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, _ *sqlite.Repository) {
		d.RootCheck = filesystem.CheckLibraryRoot
	})
	missing := filepath.Join(t.TempDir(), "gone")
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	post := func(root string) *httptest.ResponseRecorder {
		return doRequest(t, engine, http.MethodPost, "/api/v1/libraries",
			map[string]string{"name": "m", "root_path": root}, nil)
	}
	for _, root := range []string{missing, file} {
		if w := post(root); w.Code != http.StatusBadRequest {
			t.Fatalf("create %q: status = %d, want 400 (body=%s)", root, w.Code, w.Body.String())
		} else if code, _ := errorEnvelope(t, w); code != "LIBRARY_ROOT_INVALID" {
			t.Fatalf("create %q: code = %q", root, code)
		}
	}

	w := post(t.TempDir())
	if w.Code != http.StatusCreated {
		t.Fatalf("create existing dir: status = %d (body=%s)", w.Code, w.Body.String())
	}
	var lib libraryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &lib); err != nil {
		t.Fatal(err)
	}
	w = doRequest(t, engine, http.MethodPatch, "/api/v1/libraries/"+lib.ID,
		map[string]string{"root_path": t.TempDir()}, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("patch root_path: status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
}
