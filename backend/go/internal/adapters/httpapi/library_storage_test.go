package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/onsei/organizer/backend/internal/adapters/httpapi"
	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/library"
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

// A library read can finish before a simultaneous settings edit commits.
type storageSnapshotStore struct {
	*sqlite.Repository

	reads    atomic.Int32
	captured chan struct{}
	resume   chan struct{}
}

func (s *storageSnapshotStore) GetLibrary(id string) (*library.Library, error) {
	lib, err := s.Repository.GetLibrary(id)
	if s.reads.Add(1) == 1 {
		close(s.captured)
		<-s.resume
	}
	return lib, err
}

func TestLibraryPatchPreservesConcurrentEditsOfOtherFields(t *testing.T) {
	for _, tc := range []struct{ name, first, second, wantName, wantRoot string }{
		{"rename after storage", `{"name":"Renamed"}`, `{"storage_class":"ssd"}`, "Renamed", "/music"},
		{"storage after rename", `{"storage_class":"ssd"}`, `{"name":"Renamed"}`, "Renamed", "/music"},
		{"storage after root change", `{"storage_class":"ssd"}`, `{"root_path":"/moved"}`, "Music", "/moved"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			lib, err := repo.CreateLibrary("Music", "/music")
			if err != nil {
				t.Fatal(err)
			}
			store := &storageSnapshotStore{Repository: repo, captured: make(chan struct{}), resume: make(chan struct{})}
			handler := httpapi.NewServer(httpapi.Dependencies{Library: library.NewService(store, nil, repo, nil)})
			request := func(body string) *httptest.ResponseRecorder {
				out := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPatch, "/api/v1/libraries/"+lib.ID, bytes.NewBufferString(body))
				req.Header.Set("Content-Type", "application/json")
				handler.ServeHTTP(out, req)
				return out
			}
			first := make(chan *httptest.ResponseRecorder, 1)
			go func() { first <- request(tc.first) }()
			select {
			case <-store.captured:
			case <-time.After(5 * time.Second):
				t.Fatal("patch never read its snapshot")
			}
			saved := request(tc.second)
			close(store.resume)
			if saved.Code != http.StatusOK {
				t.Fatalf("second patch=%d: %s", saved.Code, saved.Body)
			}
			select {
			case response := <-first:
				if response.Code != http.StatusOK {
					t.Fatalf("first patch=%d: %s", response.Code, response.Body)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("patch did not return")
			}
			out := httptest.NewRecorder()
			handler.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/api/v1/libraries/"+lib.ID, nil))
			var got storageAPIResponse
			if err := json.Unmarshal(out.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Name != tc.wantName || got.RootPath != tc.wantRoot || got.StorageClass != "ssd" {
				t.Fatalf("concurrent edit lost: %+v", got)
			}
		})
	}
}
