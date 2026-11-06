package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/library"
)

type storageAPIResponse struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	RootPath     string               `json:"root_path"`
	StorageClass string               `json:"storage_class"`
	Libraries    []storageAPIResponse `json:"libraries"`
}

func TestLibraryStorageClassPersistsThroughAPI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	open := func() (*sqlite.Repository, http.Handler) {
		repo, err := sqlite.NewRepository(path)
		if err != nil {
			t.Fatal(err)
		}
		return repo, httpapi.NewServer(httpapi.Dependencies{Library: library.NewService(repo, nil, repo, nil)})
	}
	repo, handler := open()
	request := func(method, route, body string, status int) storageAPIResponse {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1"+route, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s: status %d, want %d: %s", method, route, w.Code, status, w.Body)
		}
		var out storageAPIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	created := request("POST", "/libraries", `{"name":"music","root_path":"/music"}`, 201)
	if created.StorageClass != "hdd" {
		t.Fatalf("new library storage class = %v, want hdd", created.StorageClass)
	}
	route := "/libraries/" + created.ID
	updated := request("PATCH", route, `{"storage_class":"ssd"}`, 200)
	if updated.StorageClass != "ssd" {
		t.Fatalf("updated library = %v", updated)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	repo, handler = open()
	defer repo.Close()
	renamed := request("PATCH", route, `{"name":"renamed"}`, 200)
	if renamed.Name != "renamed" || renamed.StorageClass != "ssd" {
		t.Fatalf("rename lost storage class: %v", renamed)
	}
	got := request("GET", route, "", 200)
	if got.StorageClass != "ssd" {
		t.Fatalf("reopened library = %v", got)
	}
	listed := request("GET", "/libraries", "", 200)
	if listed.Libraries[0].StorageClass != "ssd" {
		t.Fatalf("list = %v", listed)
	}
	for _, invalid := range []string{`"nvme"`, `""`, `1`, `null`} {
		request("PATCH", route, `{"storage_class":`+invalid+`}`, 400)
	}
	got = request("GET", route, "", 200)
	if got.StorageClass != "ssd" {
		t.Fatalf("invalid patch changed storage: %v", got)
	}
	reverted := request("PATCH", route, `{"storage_class":"hdd"}`, 200)
	if reverted.StorageClass != "hdd" {
		t.Fatalf("reverted library = %v", reverted)
	}
}
