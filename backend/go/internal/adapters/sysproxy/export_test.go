package sysproxy

import "net/url"

// RouteWindows routes target through WinHTTP-style proxy and bypass strings,
// the parsing Windows' Lookup applies to Internet Options and PAC answers.
func RouteWindows(proxy, bypass string, target *url.URL) (*url.URL, error) {
	return parseWindows(proxy, bypass).route(target)
}
