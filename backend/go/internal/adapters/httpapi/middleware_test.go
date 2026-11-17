package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
)

const (
	headerRoute = "/api/v1/libraries"
	mediaRoute  = "/api/v1/works/RJ00000001/content/copy/file.mp3"
)

// authHeader is an Authorization value; the zero value sends no header.
func authHeader(value string) map[string]string {
	if value == "" {
		return nil
	}
	return map[string]string{"Authorization": value}
}

func tokenServer(t *testing.T) http.Handler {
	t.Helper()
	return newTestServer(t, func(d *httpapi.Dependencies, _ *sqlite.Repository) { d.Token = "secret" })
}

// Accepting the right token, an absent header and the disabled-auth mode are
// asserted by TestAuthMiddleware; this lists the rejected near-misses.
func TestAuthMiddlewareViaServer(t *testing.T) {
	h := tokenServer(t)
	for _, tc := range []struct{ name, auth string }{
		{"incorrect token", "Bearer wrong"},
		{"partial token", "Bearer secre"},
		{"non-bearer scheme", "Basic c2VjcmV0"},
		{"case-sensitive scheme", "bearer secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := doRequest(t, h, http.MethodGet, headerRoute, nil, authHeader(tc.auth))
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 (auth=%q)", w.Code, tc.auth)
			}
		})
	}
}

func TestMediaAuthTakesTheTokenFromTheAddress(t *testing.T) {
	h := tokenServer(t)
	// The media routes answer an unknown work with 404 once authorized, so the
	// credential decision is whether the response is a 401.
	cases := []struct {
		name, target, auth string
		authorized         bool
	}{
		{"header token", mediaRoute, "Bearer secret", true},
		{"wrong query token", mediaRoute + "?access_token=wrong", "", false},
		{"no token", mediaRoute, "", false},
		// A header that is present decides; the query cannot rescue it.
		{"wrong header beats right query", mediaRoute + "?access_token=secret", "Bearer wrong", false},
		{"malformed header beats right query", mediaRoute + "?access_token=secret", "Basic secret", false},
		{"empty bearer beats right query", mediaRoute + "?access_token=secret", "Bearer ", false},
	}
	for _, tc := range cases {
		w := doRequest(t, h, http.MethodGet, tc.target, nil, authHeader(tc.auth))
		if (w.Code != http.StatusUnauthorized) != tc.authorized {
			t.Errorf("%s: status = %d, authorized want %v", tc.name, w.Code, tc.authorized)
		}
	}
	// An explicitly empty Authorization header still counts as present.
	w := doRequest(t, h, http.MethodGet, mediaRoute+"?access_token=secret", nil, map[string]string{"Authorization": ""})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("an explicitly empty authorization header accepted a query token: %d", w.Code)
	}
}

func TestCORSExposesResponseHeadersOnlyToAllowedOrigins(t *testing.T) {
	h := newTestServer(t, func(d *httpapi.Dependencies, _ *sqlite.Repository) {
		d.CORSOrigins = []string{"http://localhost:5173"}
	})
	for _, origin := range []string{"http://localhost:5173", "http://untrusted.example"} {
		w := doRequest(t, h, http.MethodGet, "/api/v1/health", nil, map[string]string{"Origin": origin})
		want := ""
		if origin == "http://localhost:5173" {
			want = "ETag, Retry-After"
		}
		if got := w.Header().Get("Access-Control-Expose-Headers"); got != want {
			t.Errorf("origin %s: exposed %q, want %q", origin, got, want)
		}
	}
}
