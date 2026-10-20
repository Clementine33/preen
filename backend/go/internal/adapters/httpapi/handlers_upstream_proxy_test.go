package httpapi //nolint:testpackage // white-box tests drive the routes through the real router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
)

// TestUpstreamProxyRoundTrip: direct before any save, trimmed on write, one
// stored value read back, and every refusal leaves it untouched.
func TestUpstreamProxyRoundTrip(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) { wireWorks(d, fixture) })
	read := func() string {
		return doRequest(t, engine, http.MethodGet, "/api/v1/upstream-proxy", nil, nil).Body.String()
	}
	put := func(body any) *httptest.ResponseRecorder {
		return doRequest(t, engine, http.MethodPut, "/api/v1/upstream-proxy", body, nil)
	}

	if got := read(); got != `{"url":""}` {
		t.Fatalf("initial = %s", got)
	}
	for _, c := range [][2]string{
		{" socks5://127.0.0.1:7890 ", "socks5://127.0.0.1:7890"},
		{"http://user:pw@proxy.lan", "http://user:pw@proxy.lan"},
		{"https://proxy.example/", "https://proxy.example/"},
		{"socks5h://proxy:1080", "socks5h://proxy:1080"},
	} {
		if w := put(map[string]any{"url": c[0]}); w.Code != http.StatusOK || w.Body.String() != `{"url":"`+c[1]+`"}` {
			t.Fatalf("put %q = %d %s", c[0], w.Code, w.Body.String())
		}
	}
	stored := read()

	invalid := map[string]any{
		"scheme":    map[string]any{"url": "ftp://proxy:21"},
		"no host":   map[string]any{"url": "http://"},
		"no scheme": map[string]any{"url": "proxy:8080"},
		"path":      map[string]any{"url": "http://proxy:8080/x"},
		"query":     map[string]any{"url": "http://proxy:8080?a=1"},
		"fragment":  map[string]any{"url": "http://proxy:8080#f"},
		"bad port":  map[string]any{"url": "http://proxy:port"},
		"missing":   map[string]any{},
		"null":      map[string]any{"url": nil},
	}
	for name, body := range invalid {
		if w := put(body); w.Code != http.StatusBadRequest ||
			!strings.Contains(w.Body.String(), "INVALID_UPSTREAM_PROXY") {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	if w := put(map[string]any{"url": "", "x": 1}); w.Code != http.StatusBadRequest ||
		!strings.Contains(w.Body.String(), "INVALID_ARGUMENT") {
		t.Errorf("unknown field: %d %s", w.Code, w.Body.String())
	}
	if got := read(); got != stored {
		t.Fatalf("refusals changed state: %s", got)
	}

	if w := put(map[string]any{"url": "  "}); w.Code != http.StatusOK || w.Body.String() != `{"url":""}` {
		t.Fatalf("clear = %d %s", w.Code, w.Body.String())
	}
}
