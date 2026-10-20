package works

import (
	"net/url"
	"strings"
)

// OutboundProxy is the deployment-wide outbound proxy for metadata sources and
// the cover CDN. An empty URL means a direct connection.
type OutboundProxy struct {
	URL string `json:"url"`
}

// outboundProxySchemes are the proxy protocols net/http speaks natively.
var outboundProxySchemes = map[string]bool{"http": true, "https": true, "socks5": true, "socks5h": true}

// ValidateOutboundProxy trims raw and checks it is empty or a bare proxy
// address: a supported scheme and host, optional port and userinfo, no path,
// query or fragment.
func ValidateOutboundProxy(raw string) (string, *Error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		return "", Invalid(CodeInvalidOutboundProxy, "url is not a valid proxy address")
	case !outboundProxySchemes[u.Scheme]:
		return "", Invalid(CodeInvalidOutboundProxy, "scheme must be http, https, socks5 or socks5h")
	case u.Hostname() == "":
		return "", Invalid(CodeInvalidOutboundProxy, "url must name a host")
	case (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#"):
		return "", Invalid(CodeInvalidOutboundProxy, "url must not carry a path, query or fragment")
	}
	return raw, nil
}

// OutboundProxy reads the stored proxy; never saved reads as direct.
func (s *Service) OutboundProxy() (OutboundProxy, *Error) {
	raw, err := s.store.OutboundProxy()
	if err != nil {
		return OutboundProxy{}, asInternal(err)
	}
	return OutboundProxy{URL: raw}, nil
}

// SaveOutboundProxy replaces the stored proxy (last write wins). A missing url
// is refused; nothing is stored on refusal.
func (s *Service) SaveOutboundProxy(raw *string) (OutboundProxy, *Error) {
	if raw == nil {
		return OutboundProxy{}, Invalid(CodeInvalidOutboundProxy, "url is required")
	}
	normalized, refusal := ValidateOutboundProxy(*raw)
	if refusal != nil {
		return OutboundProxy{}, refusal
	}
	if err := s.store.SaveOutboundProxy(normalized, s.now()); err != nil {
		return OutboundProxy{}, asInternal(err)
	}
	return OutboundProxy{URL: normalized}, nil
}
