package app_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/app"
	"github.com/Clementine33/preen/backend/internal/works"
)

type proxyStore struct {
	works.Store

	raw     string
	err     error
	saveErr error
}

func (s *proxyStore) OutboundProxy() (string, error) { return s.raw, s.err }

func (s *proxyStore) SaveOutboundProxy(raw string, _ time.Time) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.raw, s.err = raw, nil
	return nil
}

// An unreadable or unparsable setting is an error, never a direct connection,
// until a successful save replaces it; a failed save keeps the current route.
func TestOutboundProxyResolution(t *testing.T) {
	if u, err := app.NewOutboundProxy(&proxyStore{}).Resolve(); u != nil || err != nil {
		t.Fatalf("empty = %v, %v", u, err)
	}
	if u, err := app.NewOutboundProxy(&proxyStore{raw: "socks5://127.0.0.1:7890"}).Resolve(); err != nil ||
		u.Host != "127.0.0.1:7890" {
		t.Fatalf("set = %v, %v", u, err)
	}
	if _, err := app.NewOutboundProxy(&proxyStore{raw: "http://[bad"}).Resolve(); err == nil {
		t.Fatal("unparsable value resolved")
	}

	store := &proxyStore{err: errors.New("corrupt row")}
	p := app.NewOutboundProxy(store)
	if _, err := p.Resolve(); err == nil {
		t.Fatal("unreadable row resolved")
	}
	if err := p.SaveOutboundProxy("http://proxy:3128", time.Now()); err != nil {
		t.Fatal(err)
	}
	if u, err := p.Resolve(); err != nil || u.Host != "proxy:3128" {
		t.Fatalf("after save = %v, %v", u, err)
	}
	store.saveErr = errors.New("disk full")
	if err := p.SaveOutboundProxy("", time.Now()); err == nil {
		t.Fatal("failed save reported success")
	}
	if u, _ := p.Resolve(); u == nil || u.Host != "proxy:3128" {
		t.Fatalf("failed save changed the route to %v", u)
	}
}
