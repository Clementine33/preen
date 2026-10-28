package httpapi //nolint:testpackage // white-box tests drive the routes through the real router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
)

// TestOutboundProxyRoundTrip: system before any save, trimmed on write, one
// stored value read back, and every refusal leaves it untouched.
func TestOutboundProxyRoundTrip(t *testing.T) {
	var fixture *sqlite.Repository
	engine := newTestServer(t, func(d *Dependencies, f *sqlite.Repository) {
		wireWorks(d, f)
		fixture = f
	})
	read := func() string {
		return doRequest(t, engine, http.MethodGet, "/api/v1/outbound-proxy", nil, nil).Body.String()
	}
	put := func(body any) *httptest.ResponseRecorder {
		return doRequest(t, engine, http.MethodPut, "/api/v1/outbound-proxy", body, nil)
	}

	if got := read(); got != `{"mode":"system","url":""}` {
		t.Fatalf("initial = %s", got)
	}
	for _, c := range []struct {
		body map[string]any
		want string
	}{
		{map[string]any{"mode": "manual", "url": " socks5://127.0.0.1:7890 "}, `{"mode":"manual","url":"socks5://127.0.0.1:7890"}`},
		{map[string]any{"mode": "manual", "url": "http://user:pw@proxy.lan"}, `{"mode":"manual","url":"http://user:pw@proxy.lan"}`}, //nolint:gosec // fixture credentials
		{map[string]any{"mode": "manual", "url": "socks5h://proxy:1080"}, `{"mode":"manual","url":"socks5h://proxy:1080"}`},
		{map[string]any{"mode": "system"}, `{"mode":"system","url":""}`},
		// Outside manual mode the url is kept for switching back.
		{map[string]any{"mode": "direct", "url": "https://proxy.example/"}, `{"mode":"direct","url":"https://proxy.example/"}`},
	} {
		if w := put(c.body); w.Code != http.StatusOK || w.Body.String() != c.want {
			t.Fatalf("put %v = %d %s", c.body, w.Code, w.Body.String())
		}
	}
	stored := read()

	invalid := map[string]any{
		"no mode":     map[string]any{"url": "http://proxy:8080"},
		"bad mode":    map[string]any{"mode": "auto"},
		"manual none": map[string]any{"mode": "manual"},
		"manual null": map[string]any{"mode": "manual", "url": nil},
		"manual ''":   map[string]any{"mode": "manual", "url": "  "},
		"scheme":      map[string]any{"mode": "manual", "url": "ftp://proxy:21"},
		"no host":     map[string]any{"mode": "manual", "url": "http://"},
		"no scheme":   map[string]any{"mode": "manual", "url": "proxy:8080"},
		"path":        map[string]any{"mode": "manual", "url": "http://proxy:8080/x"},
		"query":       map[string]any{"mode": "manual", "url": "http://proxy:8080?a=1"},
		"fragment":    map[string]any{"mode": "manual", "url": "http://proxy:8080#f"},
		"bad port":    map[string]any{"mode": "manual", "url": "http://proxy:port"},
		"kept bad":    map[string]any{"mode": "direct", "url": "ftp://proxy:21"},
	}
	for name, body := range invalid {
		if w := put(body); w.Code != http.StatusBadRequest ||
			!strings.Contains(w.Body.String(), "INVALID_OUTBOUND_PROXY") {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	if w := put(map[string]any{"mode": "direct", "x": 1}); w.Code != http.StatusBadRequest ||
		!strings.Contains(w.Body.String(), "INVALID_ARGUMENT") {
		t.Errorf("unknown field: %d %s", w.Code, w.Body.String())
	}
	if got := read(); got != stored {
		t.Fatalf("refusals changed state: %s", got)
	}

	// A row saved before modes existed: a URL was manual, an empty one direct.
	for raw, want := range map[string]string{
		`{"url":"http://legacy:1"}`: `{"mode":"manual","url":"http://legacy:1"}`,
		`{"url":""}`:                `{"mode":"direct","url":""}`,
	} {
		if _, err := fixture.DB().
			Exec(`UPDATE common_settings SET settings_json = ? WHERE operation_type = 'outbound_proxy'`, raw); err != nil {
			t.Fatal(err)
		}
		if got := read(); got != want {
			t.Fatalf("legacy %s = %s", raw, got)
		}
	}
}
