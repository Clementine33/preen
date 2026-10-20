package httpapi

import "net/http"

// settingsRoutes registers the server-synced single-user settings.
func (s *Server) settingsRoutes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/preferences", protect(http.HandlerFunc(s.getPreferences)))
	mux.Handle("PUT /api/v1/preferences", protect(http.HandlerFunc(s.putPreferences)))
	mux.Handle("GET /api/v1/upstream-proxy", protect(http.HandlerFunc(s.getUpstreamProxy)))
	mux.Handle("PUT /api/v1/upstream-proxy", protect(http.HandlerFunc(s.putUpstreamProxy)))
}

// getUpstreamProxy handles GET /api/v1/upstream-proxy.
func (s *Server) getUpstreamProxy(w http.ResponseWriter, _ *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	proxy, refusal := service.UpstreamProxy()
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, proxy)
}

// putUpstreamProxy handles PUT /api/v1/upstream-proxy: last write wins.
func (s *Server) putUpstreamProxy(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	var req struct {
		URL *string `json:"url"`
	}
	if decodeErr := decodeJSON(w, r, &req); decodeErr != nil {
		writeDecodeError(w, decodeErr, "invalid upstream proxy payload")
		return
	}
	proxy, refusal := service.SaveUpstreamProxy(req.URL)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, proxy)
}
