package httpapi

import (
	"net/http"

	"github.com/Clementine33/preen/backend/internal/works"
)

// outboundProxyResponse is the GET/PUT /api/v1/outbound-proxy payload.
type outboundProxyResponse struct {
	Mode string `json:"mode" jsonschema:"enum=system,enum=manual,enum=direct"`
	URL  string `json:"url"`
}

// outboundProxyPutRequest is the PUT /api/v1/outbound-proxy payload.
type outboundProxyPutRequest struct {
	Mode string  `json:"mode" jsonschema:"enum=system,enum=manual,enum=direct"`
	URL  *string `json:"url"`
}

func toOutboundProxyResponse(proxy works.OutboundProxy) outboundProxyResponse {
	return outboundProxyResponse{Mode: proxy.Mode, URL: proxy.URL}
}

// settingsRoutes registers the server-synced single-user settings and the
// Settings page's Log Export.
func (s *Server) settingsRoutes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/preferences", protect(http.HandlerFunc(s.getPreferences)))
	mux.Handle("PUT /api/v1/preferences", protect(http.HandlerFunc(s.putPreferences)))
	mux.Handle("GET /api/v1/outbound-proxy", protect(http.HandlerFunc(s.getOutboundProxy)))
	mux.Handle("PUT /api/v1/outbound-proxy", protect(http.HandlerFunc(s.putOutboundProxy)))
	mux.Handle("GET /api/v1/diagnostics/log", protect(http.HandlerFunc(s.getDiagnosticLog)))
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
	writeJSON(w, http.StatusOK, toOutboundProxyResponse(proxy))
}

// putOutboundProxy handles PUT /api/v1/outbound-proxy: last write wins.
func (s *Server) putOutboundProxy(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	var req outboundProxyPutRequest
	if decodeErr := decodeJSON(w, r, &req); decodeErr != nil {
		writeDecodeError(w, decodeErr, "invalid outbound proxy payload")
		return
	}
	proxy, refusal := service.SaveOutboundProxy(req.Mode, req.URL)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, toOutboundProxyResponse(proxy))
}
