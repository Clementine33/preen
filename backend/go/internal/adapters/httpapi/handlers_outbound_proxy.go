package httpapi

import "net/http"

// settingsRoutes registers the server-synced single-user settings.
func (s *Server) settingsRoutes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/preferences", protect(http.HandlerFunc(s.getPreferences)))
	mux.Handle("PUT /api/v1/preferences", protect(http.HandlerFunc(s.putPreferences)))
	mux.Handle("GET /api/v1/outbound-proxy", protect(http.HandlerFunc(s.getOutboundProxy)))
	mux.Handle("PUT /api/v1/outbound-proxy", protect(http.HandlerFunc(s.putOutboundProxy)))
}

// getOutboundProxy handles GET /api/v1/outbound-proxy.
func (s *Server) getOutboundProxy(w http.ResponseWriter, _ *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	proxy, refusal := service.OutboundProxy()
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, proxy)
}

// putOutboundProxy handles PUT /api/v1/outbound-proxy: last write wins.
func (s *Server) putOutboundProxy(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	var req struct {
		URL *string `json:"url"`
	}
	if decodeErr := decodeJSON(w, r, &req); decodeErr != nil {
		writeDecodeError(w, decodeErr, "invalid outbound proxy payload")
		return
	}
	proxy, refusal := service.SaveOutboundProxy(req.URL)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, proxy)
}
