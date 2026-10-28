package sysproxy

import (
	"strings"
)

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

// parseKioslaverc reads KDE's [Proxy Settings] group: ProxyType (0 none,
// 1 manual, 2 PAC, 3 auto-detect, 4 environment) and the manual entries,
// written "http://host port" or "http://host:port".
func parseKioslaverc(content string) (proxyType string, f fixed) {
	inGroup := false
	for line := range strings.Lines(content) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inGroup = line == "[Proxy Settings]"
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !inGroup || !ok {
			continue
		}
		key, _, _ = strings.Cut(strings.TrimSpace(key), "[") // drop KConfig flags such as [$e]
		value = strings.TrimSpace(value)
		switch key {
		case "ProxyType":
			proxyType = value
		case "httpProxy":
			f.http = kdeAddr(value)
		case "httpsProxy":
			f.https = kdeAddr(value)
		case "socksProxy":
			f.socks = kdeAddr(value)
		case "NoProxyFor":
			f.bypass = splitList(value)
		case "ReversedException":
			f.reversed = value == "true"
		}
	}
	return proxyType, f
}

func kdeAddr(value string) string {
	if _, rest, ok := strings.Cut(value, "://"); ok {
		value = rest
	}
	return strings.Replace(strings.TrimSpace(value), " ", ":", 1)
}

// parseGSettings reads `gsettings list-recursively org.gnome.system.proxy`:
// mode ('none', 'manual' or 'auto'), ignore-hosts and a host and port per
// protocol.
func parseGSettings(out string) (mode string, f fixed) {
	hosts, ports := map[string]string{}, map[string]string{}
	for line := range strings.Lines(out) {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 3)
		if len(fields) != 3 {
			continue
		}
		schema, key, value := fields[0], fields[1], strings.Trim(fields[2], "'")
		protocol := strings.TrimPrefix(schema, "org.gnome.system.proxy.")
		switch {
		case schema == "org.gnome.system.proxy" && key == "mode":
			mode = value
		case schema == "org.gnome.system.proxy" && key == "ignore-hosts":
			f.bypass = splitList(strings.NewReplacer("@as", "", "[", "", "]", "", "'", "").Replace(value))
		case key == "host":
			hosts[protocol] = value
		case key == "port":
			ports[protocol] = value
		}
	}
	addr := func(protocol string) string {
		if hosts[protocol] == "" {
			return ""
		}
		return hosts[protocol] + ":" + ports[protocol]
	}
	f.http, f.https, f.socks = addr("http"), addr("https"), addr("socks")
	return mode, f
}

func splitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' || r == ' ' || r == '\t' })
}
