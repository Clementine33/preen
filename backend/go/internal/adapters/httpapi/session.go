package httpapi

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
)

func (s *Server) publicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("POST /api/v1/session", s.createSession)
	mux.HandleFunc("DELETE /api/v1/session", s.deleteSession)
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Match the login cookie, including trusted LAN HTTP.
		Name: "preen_token", Path: "/api/v1", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token *string `json:"token"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeDecodeError(w, err, "invalid session payload")
		return
	}
	if input.Token == nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "token is required")
		return
	}
	// shortcut: no login rate limit on trusted LAN; add one before supporting public deployments.
	if subtle.ConstantTimeCompare([]byte(*input.Token), []byte(s.deps.Token)) != 1 {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid access key")
		return
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Trusted LAN HTTP is supported; HTTPS requests get Secure.
		Name: "preen_token", Value: base64.RawURLEncoding.EncodeToString([]byte(s.deps.Token)),
		Path: "/api/v1", MaxAge: 34560000, HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
	w.WriteHeader(http.StatusNoContent)
}
