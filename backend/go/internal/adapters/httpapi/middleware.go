package httpapi

import (
	"crypto/subtle"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"
)

// authMiddleware accepts a Bearer header or browser cookie. An empty
// configured token disables auth entirely (local-developer mode).
func authMiddleware(token string) func(http.Handler) http.Handler {
	return tokenAuth(token, false)
}

// mediaAuthMiddleware also accepts the token as the access_token query
// parameter: an <img> or <audio> element cannot send a header, so the routes
// those elements load from take it in the address (ADR 0013).
func mediaAuthMiddleware(token string) func(http.Handler) http.Handler {
	return tokenAuth(token, true)
}

func tokenAuth(token string, fromQuery bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			const prefix = "Bearer "
			got, ok := strings.CutPrefix(r.Header.Get("Authorization"), prefix)
			fromCookie := false
			if len(r.Header.Values("Authorization")) == 0 {
				if cookie, err := r.Cookie("preen_token"); err == nil {
					decoded, decodeErr := base64.RawURLEncoding.DecodeString(cookie.Value)
					got, ok = string(decoded), decodeErr == nil
					fromCookie = true
				} else if fromQuery {
					got, ok = r.URL.Query().Get("access_token"), true
				}
			}
			if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid bearer token")
				return
			}
			if fromCookie && r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOriginWrite(r) {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "cookie write requests require the same origin")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func sameOriginWrite(r *http.Request) bool {
	if sites := r.Header.Values("Sec-Fetch-Site"); len(sites) != 0 {
		return len(sites) == 1 && sites[0] == "same-origin"
	}
	if origins := r.Header.Values("Origin"); len(origins) != 0 {
		if len(origins) != 1 {
			return false
		}
		origin, err := url.Parse(origins[0])
		return err == nil && (origin.Scheme == "http" || origin.Scheme == "https") && origin.Host == r.Host
	}
	return true
}

// corsMiddleware echoes the configured allowlisted origins. Origins outside
// the allowlist get no CORS headers (browsers then block the response).
func corsMiddleware(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		allowed[origin] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := allowed[origin]; !ok {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Expose-Headers", "ETag, Retry-After")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match, Idempotency-Key")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type responseState struct {
	http.ResponseWriter

	wroteHeader bool
	status      int
}

func (w *responseState) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	if status < http.StatusOK && status != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseState) Write(payload []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(payload)
}

func (w *responseState) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// recoveryMiddleware turns a panic into a 500 and writes the access log:
// every request at debug and a 5xx at error; a 4xx stays at debug, since it is
// an expected refusal. Only method, path, status and duration are logged; the
// query and headers can carry the access key.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := &responseState{ResponseWriter: w}
		started := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("http panic", "method", r.Method, "path", r.URL.Path,
					"panic", recovered, "stack", string(debug.Stack()))
				if !state.wroteHeader {
					writeJSON(
						state,
						http.StatusInternalServerError,
						errorResponse{Code: "INTERNAL", Message: "internal server error"},
					)
				}
				return
			}
			status := state.status
			if status == 0 {
				status = http.StatusOK
			}
			level := slog.LevelDebug
			if status >= http.StatusInternalServerError {
				level = slog.LevelError
			}
			slog.Log(r.Context(), level, "http request", "method", r.Method, "path", r.URL.Path,
				"status", status, "duration_ms", time.Since(started).Milliseconds())
		}()
		next.ServeHTTP(state, r)
	})
}
