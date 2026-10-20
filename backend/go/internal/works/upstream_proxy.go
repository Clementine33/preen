package works

import (
	"net/url"
	"strings"
)

// UpstreamProxy is the deployment-wide outbound proxy for metadata sources and
// the cover CDN. An empty URL means a direct connection.
type UpstreamProxy struct {
	URL string `json:"url"`
}

// UpstreamProxySchemes are the proxy protocols net/http speaks natively.
var upstreamProxySchemes = map[string]bool{"http": true, "https": true, "socks5": true, "socks5h": true}

// ValidateUpstreamProxy trims raw and checks it is empty or a bare proxy
// address: a supported scheme and host, optional port and userinfo, no path,
// query or fragment.
func ValidateUpstreamProxy(raw string) (string, *Error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		return "", Invalid(CodeInvalidUpstreamProxy, "url is not a valid proxy address")
	case !upstreamProxySchemes[u.Scheme]:
		return "", Invalid(CodeInvalidUpstreamProxy, "scheme must be http, https, socks5 or socks5h")
	case u.Hostname() == "":
		return "", Invalid(CodeInvalidUpstreamProxy, "url must name a host")
	case (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#"):
		return "", Invalid(CodeInvalidUpstreamProxy, "url must not carry a path, query or fragment")
	}
	return raw, nil
}

// UpstreamProxy reads the stored proxy; never saved reads as direct.
func (s *Service) UpstreamProxy() (UpstreamProxy, *Error) {
	raw, err := s.store.UpstreamProxy()
	if err != nil {
		return UpstreamProxy{}, asInternal(err)
	}
	return UpstreamProxy{URL: raw}, nil
}

// SaveUpstreamProxy replaces the stored proxy (last write wins). A missing url
// is refused; nothing is stored on refusal.
func (s *Service) SaveUpstreamProxy(raw *string) (UpstreamProxy, *Error) {
	if raw == nil {
		return UpstreamProxy{}, Invalid(CodeInvalidUpstreamProxy, "url is required")
	}
	normalized, refusal := ValidateUpstreamProxy(*raw)
	if refusal != nil {
		return UpstreamProxy{}, refusal
	}
	if err := s.store.SaveUpstreamProxy(normalized, s.now()); err != nil {
		return UpstreamProxy{}, asInternal(err)
	}
	return UpstreamProxy{URL: normalized}, nil
}
