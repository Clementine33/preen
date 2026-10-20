package app

import (
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Clementine33/preen/backend/internal/works"
)

// upstreamProxy is the works store with the Upstream Proxy held in memory:
// read once at startup, replaced after each successful save, so upstream
// requests resolve it with one atomic load. Unreadable storage is kept as an
// error, which fails requests instead of falling back to a direct connection
// until a save replaces it.
type upstreamProxy struct {
	works.Store

	mu      sync.Mutex // orders save+publish so memory matches the last stored write
	current atomic.Pointer[proxyRoute]
}

type proxyRoute struct {
	url *url.URL
	err error
}

func newUpstreamProxy(store works.Store) *upstreamProxy {
	p := &upstreamProxy{Store: store}
	p.publish(store.UpstreamProxy())
	return p
}

func (p *upstreamProxy) SaveUpstreamProxy(raw string, now time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.Store.SaveUpstreamProxy(raw, now); err != nil {
		return err
	}
	p.publish(raw, nil)
	return nil
}

// Resolve reports the proxy for one upstream request; nil means direct.
func (p *upstreamProxy) Resolve() (*url.URL, error) {
	r := p.current.Load()
	return r.url, r.err
}

func (p *upstreamProxy) publish(raw string, err error) {
	r := &proxyRoute{}
	if err == nil && raw != "" {
		r.url, err = url.Parse(raw)
	}
	if err != nil {
		r.url, r.err = nil, fmt.Errorf("upstream proxy setting is unreadable: %w", err)
	}
	p.current.Store(r)
}
