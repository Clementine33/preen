//go:build !windows

package sysproxy_test

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sysproxy"
)

var (
	cdn    = &url.URL{Scheme: "https", Host: "img.dlsite.jp", Path: "/a.jpg"}
	source = &url.URL{Scheme: "http", Host: "www.dlsite.com"}
)

func lookup(t *testing.T, target *url.URL) string {
	t.Helper()
	u, err := sysproxy.Lookup(target)
	return routed(t, u, err)
}

func TestKDEReadsKioslaverc(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	t.Setenv("XDG_CONFIG_HOME", dir)
	write := func(body string) {
		if err := os.WriteFile(filepath.Join(dir, "kioslaverc"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("[Proxy Settings]\nProxyType=1\nhttpProxy=http://127.0.0.1 7890\nhttpsProxy=http://127.0.0.1:7891\n" +
		"socksProxy=socks://127.0.0.1 7892\nNoProxyFor=localhost,.example.org\n\n[Other]\nhttpsProxy=http://wrong 1\n")
	if got := lookup(t, cdn); got != "http://127.0.0.1:7891" {
		t.Fatalf("https = %s", got)
	}
	if got := lookup(t, source); got != "http://127.0.0.1:7890" {
		t.Fatalf("http = %s", got)
	}
	if got := lookup(t, &url.URL{Scheme: "https", Host: "a.example.org"}); got != "direct" {
		t.Fatalf("bypassed = %s", got)
	}

	write("[Proxy Settings]\nProxyType=1\nhttpProxy=http://p 1\nNoProxyFor=img.dlsite.jp\nReversedException=true\n")
	if lookup(t, &url.URL{Scheme: "http", Host: "img.dlsite.jp"}) != "http://p:1" || lookup(t, source) != "direct" {
		t.Fatal("reversed exceptions must proxy only the listed hosts")
	}

	write("[Proxy Settings]\nProxyType=0\nhttpsProxy=http://p 1\n")
	if got := lookup(t, cdn); got != "direct" {
		t.Fatalf("none = %s", got)
	}

	write("[Proxy Settings]\nProxyType=2\nProxy Config Script=http://wpad/proxy.pac\n")
	if _, err := sysproxy.Lookup(cdn); err == nil {
		t.Fatal("PAC mode must fail rather than guess a route")
	}
}

func TestGNOMEReadsGSettings(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\ncat <<'OUT'\n" +
		"org.gnome.system.proxy autoconfig-url ''\n" +
		"org.gnome.system.proxy ignore-hosts ['localhost', '127.0.0.0/8', '::1', '*.example.org']\n" +
		"org.gnome.system.proxy mode 'manual'\n" +
		"org.gnome.system.proxy.http host '127.0.0.1'\n" +
		"org.gnome.system.proxy.http port 7890\n" +
		"org.gnome.system.proxy.https host ''\n" +
		"org.gnome.system.proxy.https port 0\n" +
		"org.gnome.system.proxy.socks host '127.0.0.1'\n" +
		"org.gnome.system.proxy.socks port 7891\n" +
		"OUT\n"
	if err := os.WriteFile(filepath.Join(bin, "gsettings"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
	if got := lookup(t, source); got != "http://127.0.0.1:7890" {
		t.Fatalf("http = %s", got)
	}
	// No HTTPS proxy: SOCKS carries HTTPS, as GNOME applications do.
	if got := lookup(t, cdn); got != "socks5://127.0.0.1:7891" {
		t.Fatalf("https = %s", got)
	}
	if got := lookup(t, &url.URL{Scheme: "https", Host: "a.example.org"}); got != "direct" {
		t.Fatalf("bypassed = %s", got)
	}
}

// Without a desktop session (a container, a headless server) the environment
// decides. net/http reads the environment once per process, so this must stay
// the only test that reaches that fallback.
func TestHeadlessReadsTheEnvironment(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	t.Setenv("HTTPS_PROXY", "socks5://10.0.0.2:1080")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("NO_PROXY", "")
	if got := lookup(t, cdn); got != "socks5://10.0.0.2:1080" {
		t.Fatalf("https = %s", got)
	}
	if got := lookup(t, &url.URL{Scheme: "http", Host: "127.0.0.1:37425"}); got != "direct" {
		t.Fatalf("loopback = %s", got)
	}
}
