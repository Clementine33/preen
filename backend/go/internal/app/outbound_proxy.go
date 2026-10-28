package app

import (
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Clementine33/preen/backend/internal/works"
)

// outboundProxy is the works store with the Outbound Proxy held in memory:
// read once at startup, replaced after each successful save, so upstream
// requests resolve it with one atomic load. System mode asks the operating
// system per request, so its changes apply without a save. Unreadable storage
// is kept as an error, which fails requests instead of falling back to a
// direct connection until a save replaces it.
type outboundProxy struct {
	works.Store

	system  func(target *url.URL) (*url.URL, error)
	mu      sync.Mutex // orders save+publish so memory matches the last stored write
	current atomic.Pointer[proxyRoute]
}

type proxyRoute struct {
	mode string
	url  *url.URL
	err  error
}

func newOutboundProxy(store works.Store, system func(*url.URL) (*url.URL, error)) *outboundProxy {
	p := &outboundProxy{Store: store, system: system}
	p.publish(store.OutboundProxy())
	return p
}

func (p *outboundProxy) SaveOutboundProxy(proxy works.OutboundProxy, now time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.Store.SaveOutboundProxy(proxy, now); err != nil {
		return err
	}
	p.publish(proxy, nil)
	return nil
}

// Resolve reports the proxy for one upstream request; nil means direct.
func (p *outboundProxy) Resolve(target *url.URL) (*url.URL, error) {
	r := p.current.Load()
	if r.err == nil && r.mode == works.ProxySystem {
		return p.system(target)
	}
	return r.url, r.err
}

func (p *outboundProxy) publish(proxy works.OutboundProxy, err error) {
	r := &proxyRoute{mode: proxy.Mode}
	if err == nil && proxy.Mode == works.ProxyManual {
		r.url, err = url.Parse(proxy.URL)
	}
	if err != nil {
		r.url, r.err = nil, fmt.Errorf("outbound proxy setting is unreadable: %w", err)
	}
	p.current.Store(r)
}
