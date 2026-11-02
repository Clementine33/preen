package sysproxy

import "strings"

// parseWindows reads WinHTTP's proxy and bypass strings: "host:port" for
// every protocol, or "http=host:port;https=…;socks=…". The first entry for a
// scheme wins, as in a PAC answer listing fallbacks.
//
// shortcut: Windows' "socks=" is SOCKS4 to browsers but is dialled as SOCKS5
// here, since Go speaks only SOCKS5; mixed-port proxy tools accept both.
// Revisit if a SOCKS4-only proxy is reported.
func parseWindows(proxy, bypass string) fixed {
	var f fixed
	set := func(slot *string, addr string) {
		if *slot == "" {
			*slot = addr
		}
	}
	for _, entry := range splitList(proxy) {
		scheme, addr, ok := strings.Cut(entry, "=")
		if !ok {
			scheme, addr = "", entry
		}
		if prefix, rest, ok := strings.Cut(addr, "://"); ok {
			addr = rest
			if strings.HasPrefix(prefix, "socks") {
				scheme = "socks"
			}
		}
		switch strings.ToLower(scheme) {
		case "":
			set(&f.http, addr)
			set(&f.https, addr)
		case "http":
			set(&f.http, addr)
		case "https":
			set(&f.https, addr)
		case "socks":
			set(&f.socks, addr)
		}
	}
	f.bypass = splitList(bypass)
	return f
}

func splitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' || r == ' ' || r == '\t' })
}
