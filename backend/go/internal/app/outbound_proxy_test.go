package app_test

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/app"
	"github.com/Clementine33/preen/backend/internal/works"
)

type proxyStore struct {
	works.Store

	stored  works.OutboundProxy
	err     error
	saveErr error
}

func (s *proxyStore) OutboundProxy() (works.OutboundProxy, error) { return s.stored, s.err }

func (s *proxyStore) SaveOutboundProxy(proxy works.OutboundProxy, _ time.Time) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.stored, s.err = proxy, nil
	return nil
}

var target = &url.URL{Scheme: "https", Host: "www.dlsite.com"}

// systemProxy answers like the operating system would, recording what it was asked.
func systemProxy(asked *[]string) func(*url.URL) (*url.URL, error) {
	return func(u *url.URL) (*url.URL, error) {
		*asked = append(*asked, u.Host)
		return &url.URL{Scheme: "http", Host: "system:8080"}, nil
	}
}

// An unreadable or unparsable setting is an error, never a direct connection,
// until a successful save replaces it; a failed save keeps the current route.
func TestOutboundProxyResolution(t *testing.T) {
	var asked []string
	system := systemProxy(&asked)
	resolve := func(stored works.OutboundProxy) (*url.URL, error) {
		return app.NewOutboundProxy(&proxyStore{stored: stored}, system).Resolve(target)
	}
	if u, err := resolve(works.OutboundProxy{Mode: works.ProxyDirect, URL: "http://kept:1"}); u != nil || err != nil {
		t.Fatalf("direct = %v, %v", u, err)
	}
	if u, err := resolve(works.OutboundProxy{Mode: works.ProxyManual, URL: "socks5://127.0.0.1:7890"}); err != nil ||
		u.Host != "127.0.0.1:7890" {
		t.Fatalf("manual = %v, %v", u, err)
	}
	if u, err := resolve(works.OutboundProxy{Mode: works.ProxySystem}); err != nil || u.Host != "system:8080" ||
		len(asked) != 1 || asked[0] != "www.dlsite.com" {
		t.Fatalf("system = %v, %v, asked %v", u, err, asked)
	}
	if _, err := resolve(works.OutboundProxy{Mode: works.ProxyManual, URL: "http://[bad"}); err == nil {
		t.Fatal("unparsable value resolved")
	}

	store := &proxyStore{err: errors.New("corrupt row")}
	p := app.NewOutboundProxy(store, system)
	if _, err := p.Resolve(target); err == nil || len(asked) != 1 {
		t.Fatalf("unreadable row resolved (asked system %v)", asked)
	}
	if err := p.SaveOutboundProxy(
		works.OutboundProxy{Mode: works.ProxyManual, URL: "http://proxy:3128"},
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	if u, err := p.Resolve(target); err != nil || u.Host != "proxy:3128" {
		t.Fatalf("after save = %v, %v", u, err)
	}
	store.saveErr = errors.New("disk full")
	if err := p.SaveOutboundProxy(works.OutboundProxy{Mode: works.ProxyDirect}, time.Now()); err == nil {
		t.Fatal("failed save reported success")
	}
	if u, _ := p.Resolve(target); u == nil || u.Host != "proxy:3128" {
		t.Fatalf("failed save changed the route to %v", u)
	}
}
