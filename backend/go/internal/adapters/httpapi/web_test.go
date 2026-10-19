package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
)

func TestWebCacheAndContainment(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"index.html": "app", "assets/main-B5jKx_7a.js": "hashed", "favicon.svg": "icon"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	handler := httpapi.NewServer(httpapi.Dependencies{WebRoot: root, Token: "secret"})
	for _, tc := range []struct {
		path, cache string
		status      int
	}{
		{"/assets/main-B5jKx_7a.js", "public, max-age=31536000, immutable", 200},
		{"/assets/", "no-cache", 200},
		{"/favicon.svg", "no-cache", 200},
		{"/../index.html", "", 404},
		{"/%2e%2e/index.html", "", 404},
		{"/missing/../index.html", "", 404},
		{"/missing%2f..%2findex.html", "", 404},
		{"/..%5cindex.html", "", 404},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if response.Code != tc.status || response.Header().Get("Cache-Control") != tc.cache {
			t.Errorf("%s: status %d, cache %q", tc.path, response.Code, response.Header().Get("Cache-Control"))
		}
	}
}

func TestWebRejectsSymlinksOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "secret.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	response := httptest.NewRecorder()
	httpapi.NewServer(httpapi.Dependencies{WebRoot: root}).
		ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/secret.txt", nil))
	if response.Code != 404 || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
}

func TestWebDisabledPreservesNonAPIResponse(t *testing.T) {
	response := httptest.NewRecorder()
	httpapi.NewServer(httpapi.Dependencies{}).
		ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/works/RJ123456", nil))
	if response.Code != 404 || response.Body.String() != "404 page not found\n" {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
}

func TestWebPreservesAPIErrors(t *testing.T) {
	for _, web := range []bool{false, true} {
		deps := httpapi.Dependencies{Token: "secret"}
		if web {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = root.Close() })
			deps.WebRoot = root
		}
		handler := httpapi.NewServer(deps)
		for _, tc := range []struct {
			method, path, code string
			status             int
		}{
			{"GET", "/api/v1/unknown", "NOT_FOUND", 404},
			{"POST", "/api/v1/health", "METHOD_NOT_ALLOWED", 405},
			{"PUT", "/api/v1/health", "METHOD_NOT_ALLOWED", 405},
			{"HEAD", "/api/v1/health", "METHOD_NOT_ALLOWED", 405},
			{"GET", "/api/v1/libraries", "UNAUTHORIZED", 401},
		} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			var envelope struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("%s %s: %v (%s)", tc.method, tc.path, err, response.Body.String())
			}
			if response.Code != tc.status || envelope.Code != tc.code || envelope.Message == "" {
				t.Errorf("%s %s: status %d, error %+v", tc.method, tc.path, response.Code, envelope)
			}
		}
	}
}

func TestWebServesFilesAndHistoryWithoutAuthentication(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>preen</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	handler := httpapi.NewServer(httpapi.Dependencies{Token: "secret", WebRoot: root})
	for _, target := range []string{"/", "/index.html", "/works/RJ123456"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusOK || response.Body.String() != "<html>preen</html>" {
			t.Fatalf("%s: status %d, body %q", target, response.Code, response.Body.String())
		}
		if response.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("%s: index must be revalidated", target)
		}
	}
}
