package httpapi //nolint:testpackage // white-box tests exercise unexported internals

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddlewareDirect(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	run := func(h http.Handler, auth string) int {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	t.Run("empty configured token disables auth", func(t *testing.T) {
		if got := run(authMiddleware("")(ok), ""); got != http.StatusOK {
			t.Fatalf("status = %d, want 200 with empty token config", got)
		}
	})

	h := authMiddleware("secret")(ok)
	cases := []struct {
		name string
		auth string
		want int
	}{
		{name: "correct token", auth: "Bearer secret", want: http.StatusOK},
		{name: "incorrect token", auth: "Bearer wrong", want: http.StatusUnauthorized},
		{name: "missing header", auth: "", want: http.StatusUnauthorized},
		{name: "partial token", auth: "Bearer secre", want: http.StatusUnauthorized},
		{name: "non-bearer scheme", auth: "Basic c2VjcmV0", want: http.StatusUnauthorized},
		{name: "case-sensitive scheme", auth: "bearer secret", want: http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(h, tc.auth); got != tc.want {
				t.Fatalf("status = %d, want %d (auth=%q)", got, tc.want, tc.auth)
			}
		})
	}
}

func TestMediaAuthTakesTheTokenFromTheAddress(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	run := func(h http.Handler, target, auth string) int {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	media := mediaAuthMiddleware("secret")(ok)
	cases := []struct {
		name, target, auth string
		want               int
	}{
		{"query token", "/?access_token=secret", "", http.StatusOK},
		{"header token", "/", "Bearer secret", http.StatusOK},
		{"wrong query token", "/?access_token=wrong", "", http.StatusUnauthorized},
		{"no token", "/", "", http.StatusUnauthorized},
		// A header that is present decides; the query cannot rescue it.
		{"wrong header beats right query", "/?access_token=secret", "Bearer wrong", http.StatusUnauthorized},
		{"malformed header beats right query", "/?access_token=secret", "Basic secret", http.StatusUnauthorized},
		{"empty bearer beats right query", "/?access_token=secret", "Bearer ", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		if got := run(media, tc.target, tc.auth); got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
	// The ordinary routes never read the address.
	if got := run(authMiddleware("secret")(ok), "/?access_token=secret", ""); got != http.StatusUnauthorized {
		t.Errorf("header-only route accepted a query token: %d", got)
	}
	r := httptest.NewRequest(http.MethodGet, "/?access_token=secret", nil)
	r.Header.Set("Authorization", "")
	w := httptest.NewRecorder()
	media.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("an explicitly empty authorization header accepted a query token: %d", w.Code)
	}
}

func TestCORSExposesRetryAfterOnlyToAllowedOrigins(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	h := corsMiddleware([]string{"http://localhost:5173"})(handler)
	for _, origin := range []string{"http://localhost:5173", "http://untrusted.example"} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		want := ""
		if origin == "http://localhost:5173" {
			want = "Retry-After"
		}
		if got := w.Header().Get("Access-Control-Expose-Headers"); got != want {
			t.Errorf("origin %s: exposed %q, want %q", origin, got, want)
		}
	}
}
