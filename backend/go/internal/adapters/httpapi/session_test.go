package httpapi_test

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
)

func TestBrowserLogout(t *testing.T) {
	server := httpapi.NewServer(httpapi.Dependencies{Token: "secret"})
	for _, credential := range []string{"", "preen_token=invalid", "preen_token=c2VjcmV0"} {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/session", nil)
		req.Header.Set("Cookie", credential)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, req)
		cookies := response.Result().Cookies()
		if response.Code != http.StatusNoContent || len(cookies) != 1 {
			t.Fatalf("logout = %d, cookies %v", response.Code, cookies)
		}
		cookie := cookies[0]
		if cookie.Name != "preen_token" || cookie.Value != "" || cookie.Path != "/api/v1" ||
			cookie.MaxAge != -1 || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
			t.Fatalf("logout cookie = %+v", cookie)
		}
	}
}

func TestCookieWritesRequireSameOrigin(t *testing.T) {
	server := httpapi.NewServer(httpapi.Dependencies{Token: "secret"})
	for _, tc := range []struct {
		name, site, origin, authorization string
		status                            int
	}{
		{"same origin", "same-origin", "http://other:8765", "", 400},
		{"same site", "same-site", "http://example.com:8765", "", 403},
		{"cross site", "cross-site", "", "", 403},
		{"none site", "none", "", "", 403},
		{"origin matches", "", "http://example.com:8765", "", 400},
		{"origin other port", "", "http://example.com:8766", "", 403},
		{"origin other host", "", "http://other:8765", "", 403},
		{"null origin", "", "null", "", 403},
		{"malformed origin", "", "://example.com:8765", "", 403},
		{"no browser headers", "", "", "", 400},
		{"bearer exempt", "cross-site", "http://other:8765", "Bearer secret", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodPost, "http://example.com:8765/api/v1/libraries", bytes.NewBufferString("{"),
			)
			req.Header.Set("Cookie", "preen_token=c2VjcmV0")
			if tc.site != "" {
				req.Header.Set("Sec-Fetch-Site", tc.site)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.authorization != "" {
				req.Header.Set("Authorization", tc.authorization)
			}
			response := httptest.NewRecorder()
			server.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.status, response.Body)
			}
		})
	}
}

func TestCookieReadsIgnoreOrigin(t *testing.T) {
	server := httpapi.NewServer(httpapi.Dependencies{Token: "secret"})
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req := httptest.NewRequest(method, "/api/v1/libraries", nil)
		req.Header.Set("Cookie", "preen_token=c2VjcmV0")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set("Origin", "http://other:8766")
		response := httptest.NewRecorder()
		server.ServeHTTP(response, req)
		if response.Code == http.StatusForbidden || response.Code == http.StatusUnauthorized {
			t.Fatalf("%s read refused: %d", method, response.Code)
		}
	}
}

func TestBrowserLogin(t *testing.T) {
	const token = `shared;"\secret`
	server := httpapi.NewServer(httpapi.Dependencies{Token: token})
	body, _ := json.Marshal(map[string]string{"token": token})
	login := httptest.NewRecorder()
	server.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewReader(body)))
	if login.Code != http.StatusNoContent {
		t.Fatalf("login status = %d: %s", login.Code, login.Body)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %v", cookies)
	}
	cookie := cookies[0]
	if cookie.Value != "c2hhcmVkOyJcc2VjcmV0" || !cookie.HttpOnly ||
		cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/api/v1" ||
		cookie.MaxAge != 34560000 || cookie.Secure {
		t.Fatalf("cookie attributes = %+v", cookie)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/libraries", nil)
	req.AddCookie(cookie)
	authenticated := httptest.NewRecorder()
	server.ServeHTTP(authenticated, req)
	if authenticated.Code == http.StatusUnauthorized {
		t.Fatal("login cookie rejected")
	}
	wrong := httptest.NewRecorder()
	server.ServeHTTP(
		wrong,
		httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewBufferString(`{"token":"wrong"}`)),
	)
	if wrong.Code != http.StatusUnauthorized || len(wrong.Result().Cookies()) != 0 {
		t.Fatalf("failed login = %d, cookies %v", wrong.Code, wrong.Result().Cookies())
	}
}

func TestBrowserCredentialPrecedence(t *testing.T) {
	server := httpapi.NewServer(httpapi.Dependencies{Token: "secret"})
	for _, tc := range []struct {
		name, path, bearer, cookie string
		authorized                 bool
	}{
		{"cookie", "/api/v1/libraries", "", "c2VjcmV0", true},
		{"invalid cookie", "/api/v1/libraries", "", "%%%", false},
		{"wrong cookie", "/api/v1/libraries", "", "d3Jvbmc", false},
		{"bearer beats cookie", "/api/v1/libraries", "Bearer secret", "d3Jvbmc", true},
		{"bad bearer blocks cookie", "/api/v1/libraries", "Bearer wrong", "c2VjcmV0", false},
		{"malformed bearer blocks cookie", "/api/v1/libraries", "Basic secret", "c2VjcmV0", false},
		{"ordinary query refused", "/api/v1/libraries?access_token=secret", "", "", false},
		{"media query", "/api/v1/works/RJ00000001/content/copy/file.mp3?access_token=secret", "", "", true},
		{"cookie beats query", "/api/v1/works/RJ00000001/content/copy/file.mp3?access_token=wrong", "", "c2VjcmV0", true},
		{"bad cookie blocks query", "/api/v1/works/RJ00000001/content/copy/file.mp3?access_token=secret", "", "d3Jvbmc", false},
		{"thumbnail query", "/api/v1/works/RJ00000001/thumbs/copy/cover.jpg?h=120&access_token=secret", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.bearer != "" {
				req.Header.Set("Authorization", tc.bearer)
			}
			if tc.cookie != "" {
				req.Header.Set("Cookie", "preen_token="+tc.cookie)
			}
			response := httptest.NewRecorder()
			server.ServeHTTP(response, req)
			if (response.Code != http.StatusUnauthorized) != tc.authorized {
				t.Fatalf("status = %d", response.Code)
			}
		})
	}
}

func TestBrowserLoginHTTPS(t *testing.T) {
	server := httpapi.NewServer(httpapi.Dependencies{Token: "secret"})
	for _, forwarded := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewBufferString(`{"token":"secret"}`))
		if forwarded {
			req.Header.Set("X-Forwarded-Proto", "https")
		} else {
			req.TLS = &tls.ConnectionState{}
		}
		response := httptest.NewRecorder()
		server.ServeHTTP(response, req)
		if response.Code != http.StatusNoContent || !response.Result().Cookies()[0].Secure {
			t.Fatalf("HTTPS cookie missing: %d", response.Code)
		}
	}
}

func TestBrowserLoginRejectsMalformedPayloads(t *testing.T) {
	server := httpapi.NewServer(httpapi.Dependencies{Token: "secret"})
	for _, body := range []string{`{"token":"secret","extra":1}`, `{"token":1}`, `{"token":"secret"} {}`, `null`, `{}`} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewBufferString(body)))
		if response.Code != http.StatusBadRequest || len(response.Result().Cookies()) != 0 {
			t.Fatalf("invalid payload accepted: %d", response.Code)
		}
	}
}
