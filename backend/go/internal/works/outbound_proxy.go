package works

import (
	"net/url"
	"strings"
)

// Outbound Proxy modes. System follows the operating system's proxy settings
// for each upstream request, so a Desktop Shell's backend connects the way
// its WebView does (ADR 0019); it is what a never-saved setting reads as.
const (
	ProxySystem = "system"
	ProxyManual = "manual"
	ProxyDirect = "direct"
)

// OutboundProxy is the deployment-wide outbound proxy for metadata sources and
// the cover CDN. URL is used only in manual mode and is kept otherwise, so
// switching back restores it.
type OutboundProxy struct {
	Mode string `json:"mode"`
	URL  string `json:"url"`
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

// OutboundProxy reads the stored proxy.
func (s *Service) OutboundProxy() (OutboundProxy, *Error) {
	proxy, err := s.store.OutboundProxy()
	if err != nil {
		return OutboundProxy{}, asInternal(err)
	}
	return proxy, nil
}

// SaveOutboundProxy replaces the stored proxy (last write wins). Manual mode
// needs a url; nothing is stored on refusal.
func (s *Service) SaveOutboundProxy(mode string, raw *string) (OutboundProxy, *Error) {
	if mode != ProxySystem && mode != ProxyManual && mode != ProxyDirect {
		return OutboundProxy{}, Invalid(CodeInvalidOutboundProxy, "mode must be system, manual or direct")
	}
	proxy := OutboundProxy{Mode: mode}
	if raw != nil {
		normalized, refusal := ValidateOutboundProxy(*raw)
		if refusal != nil {
			return OutboundProxy{}, refusal
		}
		proxy.URL = normalized
	}
	if mode == ProxyManual && proxy.URL == "" {
		return OutboundProxy{}, Invalid(CodeInvalidOutboundProxy, "manual mode needs a url")
	}
	if err := s.store.SaveOutboundProxy(proxy, s.now()); err != nil {
		return OutboundProxy{}, asInternal(err)
	}
	return proxy, nil
}
