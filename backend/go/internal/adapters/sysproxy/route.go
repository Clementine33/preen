// Package sysproxy answers which proxy the operating system would use for one
// URL, so the backend can connect the way a Desktop Shell's WebView does
// (ADR 0019). Windows asks WinHTTP, which also runs PAC scripts and
// auto-detection; Linux reads the desktop's own settings (KDE kioslaverc,
// otherwise GNOME gsettings) and falls back to HTTP(S)_PROXY / NO_PROXY.
// A nil URL means direct. Loopback targets are always direct.
package sysproxy

import (
	"net"
	"net/netip"
	"net/url"
	"path"
	"strings"
)

// fixed is one manually configured proxy set: a host:port per target scheme
// ("" when unset) and the hosts that bypass it.
type fixed struct {
	http, https, socks string
	bypass             []string
	// reversed proxies only the bypass list (KDE's ReversedException).
	reversed bool
}

// route picks the proxy for target: the scheme's own proxy, else SOCKS, else
// direct, the order browsers apply to per-protocol settings.
func (f fixed) route(target *url.URL) (*url.URL, error) {
	host := target.Hostname()
	if loopback(host) || bypassed(host, f.bypass) != f.reversed {
		return nil, nil
	}
	addr := f.http
	if target.Scheme == "https" {
		addr = f.https
	}
	scheme := "http"
	if addr == "" && f.socks != "" {
		addr, scheme = f.socks, "socks5"
	}
	if addr == "" {
		return nil, nil
	}
	return url.Parse(scheme + "://" + addr)
}

func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

// bypassed matches host against bypass rules as the platforms write them:
// exact names, "*.suffix" / ".suffix" globs, CIDR ranges, "*" and Windows'
// "<local>" (a name without a dot). A rule's scheme and port are ignored.
func bypassed(host string, rules []string) bool {
	host = strings.ToLower(host)
	ip, ipErr := netip.ParseAddr(host)
	for _, rule := range rules {
		rule = strings.ToLower(strings.TrimSpace(rule))
		if _, rest, ok := strings.Cut(rule, "://"); ok {
			rule = rest
		}
		if h, _, err := net.SplitHostPort(rule); err == nil {
			rule = h
		}
		switch {
		case rule == "":
		case rule == "<local>":
			if ipErr != nil && !strings.Contains(host, ".") {
				return true
			}
		case strings.Contains(rule, "/"):
			if prefix, err := netip.ParsePrefix(rule); err == nil && ipErr == nil && prefix.Contains(ip) {
				return true
			}
		case strings.HasPrefix(rule, "."):
			if host == rule[1:] || strings.HasSuffix(host, rule) {
				return true
			}
		case strings.HasPrefix(rule, "*."):
			if host == rule[2:] || strings.HasSuffix(host, rule[1:]) {
				return true
			}
		default:
			if matched, _ := path.Match(strings.Trim(rule, "[]"), host); matched {
				return true
			}
		}
	}
	return false
}
