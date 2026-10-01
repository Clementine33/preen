package httpapi

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strings"
)

// authMiddleware enforces a static bearer token on protected routes. An empty
// configured token disables auth entirely (local-developer mode).
func authMiddleware(token string) func(http.Handler) http.Handler {
	return tokenAuth(token, false)
}

// mediaAuthMiddleware also accepts the token as the access_token query
// parameter: an <img> or <audio> element cannot send a header, so the routes
// those elements load from take it in the address (ADR 0016 §3).
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
			if len(r.Header.Values("Authorization")) == 0 && fromQuery {
				got, ok = r.URL.Query().Get("access_token"), true
			}
			if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid bearer token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
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
			w.Header().Set("Access-Control-Expose-Headers", "Retry-After")
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
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseState) Write(payload []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(payload)
}

func (w *responseState) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := &responseState{ResponseWriter: w}
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("http panic: %v", recovered)
				if !state.wroteHeader {
					writeJSON(
						state,
						http.StatusInternalServerError,
						errorResponse{Code: "INTERNAL", Message: "internal server error"},
					)
				}
			}
		}()
		next.ServeHTTP(state, r)
	})
}
