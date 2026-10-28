package sysproxy_test

import (
	"net/url"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sysproxy"
)

// routed renders the proxy chosen for target, "direct" for none.
func routed(t *testing.T, u *url.URL, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if u == nil {
		return "direct"
	}
	return u.String()
}

func TestWindowsProxyStrings(t *testing.T) {
	https := &url.URL{Scheme: "https", Host: "img.dlsite.jp"}
	for _, c := range []struct{ proxy, bypass, target, want string }{
		{"127.0.0.1:7890", "", "https://img.dlsite.jp/a", "http://127.0.0.1:7890"},
		{"http=h:1;https=s:2;socks=k:3", "", "https://img.dlsite.jp/a", "http://s:2"},
		{"http=h:1;https=s:2", "", "http://www.dlsite.com/", "http://h:1"},
		{"http=h:1;socks=k:3", "", "https://img.dlsite.jp/a", "socks5://k:3"},
		{"http=h:1", "", "https://img.dlsite.jp/a", "direct"},
		{"socks5://k:3", "", "https://img.dlsite.jp/a", "socks5://k:3"},
		// A PAC answer lists fallbacks; the first one is used.
		{"first:1; second:2", "", "https://img.dlsite.jp/a", "http://first:1"},
		{"p:1", "<local>;*.dlsite.jp", "https://img.dlsite.jp/a", "direct"},
		{"p:1", "<local>", "http://intranet/", "direct"},
		{"p:1", "dlsite.jp", "https://img.dlsite.jp/a", "http://p:1"},
		{"p:1", "10.0.0.0/8", "http://10.1.2.3/", "direct"},
		{"p:1", "", "http://127.0.0.1:37425/api", "direct"},
		{"p:1", "", "http://localhost:5173/", "direct"},
		{"", "", https.String(), "direct"},
	} {
		target, _ := url.Parse(c.target)
		u, err := sysproxy.RouteWindows(c.proxy, c.bypass, target)
		if got := routed(t, u, err); got != c.want {
			t.Errorf("%q bypass %q for %s = %s, want %s", c.proxy, c.bypass, c.target, got, c.want)
		}
	}
}
