// Package coverhttp downloads only validated public cover CDN resources.
//
// The host allowlist and redirect checks always apply. Direct connections also
// refuse non-public addresses (dialPublic). Through the Upstream Proxy that
// dial check is skipped, since the administrator-chosen proxy may sit on
// loopback or the LAN and resolves the CDN host itself; the proxy comes only
// from the user setting, never from HTTP(S)_PROXY in the environment.
package coverhttp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"time"

	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

type Client struct{ http *http.Client }

func New(client *http.Client) *Client {
	c := http.Client{Timeout: 10 * time.Second}
	if client != nil {
		c = *client
		if c.Timeout == 0 {
			c.Timeout = 10 * time.Second
		}
	}
	c.Jar = nil
	if c.Transport == nil {
		tr := (&http.Transport{ForceAttemptHTTP2: true}).Clone()
		tr.Proxy = nil
		tr.DialContext = dialPublic
		c.Transport = tr
	}
	c.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return allowed(r.URL)
	}
	return &Client{http: &c}
}

// NewProxied sends requests through the proxy that proxy reports for each
// request, direct when it reports nil. An error fails the request, never a
// direct fallback. The host allowlist and redirect checks apply either way;
// the public-address dial check applies only to direct connections, because
// the proxy address is chosen by the administrator and may be on loopback or
// the LAN, and the proxy, not this process, then resolves the CDN host.
func NewProxied(proxy func() (*url.URL, error)) *Client {
	direct := (&http.Transport{ForceAttemptHTTP2: true}).Clone()
	direct.DialContext = dialPublic
	proxied := (&http.Transport{ForceAttemptHTTP2: true}).Clone()
	proxied.Proxy = func(r *http.Request) (*url.URL, error) {
		u, _ := r.Context().Value(proxyKey{}).(*url.URL)
		return u, nil
	}
	return New(&http.Client{Transport: routed{proxy: proxy, direct: direct, proxied: proxied}})
}

type proxyKey struct{}

// routed resolves the proxy once per request and hands that answer to the
// transport, so a setting changed mid-request cannot pair a proxy with the
// direct-dial check or the reverse.
type routed struct {
	proxy           func() (*url.URL, error)
	direct, proxied *http.Transport
}

func (t routed) RoundTrip(r *http.Request) (*http.Response, error) {
	u, err := t.proxy()
	if err != nil {
		return nil, err
	}
	if u == nil {
		return t.direct.RoundTrip(r)
	}
	return t.proxied.RoundTrip(r.WithContext(context.WithValue(r.Context(), proxyKey{}, u)))
}

func allowed(u *url.URL) error {
	if u.Scheme != "https" || u.Hostname() != "img.dlsite.jp" || u.User != nil ||
		(u.Port() != "" && u.Port() != "443") {
		return coverdelivery.Failure("UNSUPPORTED_CDN", fmt.Errorf("unsupported cover host"))
	}
	return nil
}
func public(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, raw := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "64:ff9b::/96"} {
		if netip.MustParsePrefix(raw).Contains(ip) {
			return false
		}
	}
	return true
}
func dialPublic(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("empty DNS response")
	}
	for _, ip := range ips {
		if !public(ip) {
			return nil, coverdelivery.Failure("UNSUPPORTED_CDN", fmt.Errorf("nonpublic CDN address"))
		}
	}
	var last error
	d := net.Dialer{Timeout: 10 * time.Second}
	for _, ip := range ips {
		conn, e := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return conn, nil
		}
		last = e
	}
	return nil, last
}
func (c *Client) Download(ctx context.Context, raw string, v coverdelivery.Validators) (coverdelivery.Download, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return coverdelivery.Download{}, coverdelivery.Failure("UNSUPPORTED_CDN", err)
	}
	if err = allowed(u); err != nil {
		return coverdelivery.Download{}, err
	}
	for attempt := range 3 {
		result, retry, delay, e := c.once(ctx, raw, v)
		if e == nil || !retry || attempt == 2 {
			return result, e
		}
		if delay <= 0 {
			delay = time.Duration(attempt+1) * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return coverdelivery.Download{}, ctx.Err()
		case <-timer.C:
		}
	}
	return coverdelivery.Download{}, fmt.Errorf("download failed")
}

func (c *Client) once(
	ctx context.Context,
	raw string,
	v coverdelivery.Validators,
) (coverdelivery.Download, bool, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return coverdelivery.Download{}, false, 0, err
	}
	req.Header.Set("Accept", "image/webp,image/png,image/jpeg,image/gif")
	if v.ETag != "" {
		req.Header.Set("If-None-Match", v.ETag)
	}
	if v.LastModified != "" {
		req.Header.Set("If-Modified-Since", v.LastModified)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return coverdelivery.Download{}, ctx.Err() == nil && coverdelivery.Code(err) != "UNSUPPORTED_CDN", 0, err
	}
	defer res.Body.Close()
	out := coverdelivery.Download{ETag: res.Header.Get("ETag"), LastModified: res.Header.Get("Last-Modified")}
	if res.StatusCode == http.StatusNotModified && (v.ETag != "" || v.LastModified != "") {
		out.NotModified = true
		return out, false, 0, nil
	}
	if res.StatusCode != http.StatusOK {
		delay := time.Duration(0)
		if n, e := strconv.Atoi(res.Header.Get("Retry-After")); e == nil && n > 0 {
			delay = time.Duration(min(n, 300)) * time.Second
		} else if until, e := http.ParseTime(res.Header.Get("Retry-After")); e == nil {
			delay = min(time.Until(until), 5*time.Minute)
		}
		retry := res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests
		return out, retry, delay, coverdelivery.Failure("DOWNLOAD_FAILED", fmt.Errorf("CDN HTTP %d", res.StatusCode))
	}
	if res.ContentLength > coverdelivery.MaxBytes {
		return out, false, 0, coverdelivery.Failure("IMAGE_TOO_LARGE", fmt.Errorf("cover exceeds size limit"))
	}
	out.Data, err = io.ReadAll(io.LimitReader(res.Body, coverdelivery.MaxBytes+1))
	if err != nil {
		return out, true, 0, err
	}
	if len(out.Data) > coverdelivery.MaxBytes {
		return out, false, 0, coverdelivery.Failure("IMAGE_TOO_LARGE", fmt.Errorf("cover exceeds size limit"))
	}
	out.Extension, out.ContentType, err = coverdelivery.Inspect(out.Data)
	return out, false, 0, err
}
