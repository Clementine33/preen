package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// sourceGate bounds concurrent requests and request starts across all callers.
type sourceGate struct {
	slots              chan struct{}
	interval           time.Duration
	mu                 sync.Mutex
	next, blockedUntil time.Time
}

func newSourceGate(concurrency int, interval time.Duration) *sourceGate {
	return &sourceGate{slots: make(chan struct{}, concurrency), interval: interval}
}

func (g *sourceGate) acquire(ctx context.Context) (func(), error) {
	select {
	case g.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	release := func() { <-g.slots }
	g.mu.Lock()
	now := time.Now()
	if now.Before(g.blockedUntil) {
		g.mu.Unlock()
		release()
		return nil, fmt.Errorf("source is cooling down")
	}
	start := now
	if g.next.After(start) {
		start = g.next
	}
	g.next = start.Add(g.interval)
	g.mu.Unlock()
	if err := pause(ctx, time.Until(start)); err != nil {
		release()
		return nil, err
	}
	g.mu.Lock()
	blocked := time.Now().Before(g.blockedUntil)
	g.mu.Unlock()
	if blocked {
		release()
		return nil, fmt.Errorf("source is cooling down")
	}
	return release, nil
}

func (g *sourceGate) coolDown(header string) {
	until := time.Now().Add(time.Minute)
	if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
		until = until.Add(time.Duration(seconds)*time.Second - time.Minute)
	} else if date, err := http.ParseTime(header); err == nil && date.After(until) {
		until = date
	}
	if minimum := time.Now().Add(time.Minute); minimum.After(until) {
		until = minimum
	}
	g.mu.Lock()
	if until.After(g.blockedUntil) {
		g.blockedUntil = until
	}
	g.mu.Unlock()
}

func pause(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

func (c *Client) get(ctx context.Context, address string, out any) error {
	u, err := url.Parse(address)
	if err != nil {
		return err
	}
	gate := c.dlsite
	if u.Host == "api.asmr.one" {
		gate = c.asmr
	} else if u.Host != "www.dlsite.com" {
		return fmt.Errorf("unsupported source host")
	}
	if u.Scheme != "https" {
		return fmt.Errorf("source requires HTTPS")
	}
	for attempt := range 3 {
		body, retry, requestErr := c.request(ctx, address, gate)
		if requestErr == nil {
			return json.Unmarshal(body, out)
		}
		if !retry || attempt == 2 {
			return requestErr
		}
		if err := pause(
			ctx,
			time.Duration(1<<attempt)*time.Second+time.Duration(rand.IntN(250))*time.Millisecond,
		); err != nil {
			return err
		}
	}
	return fmt.Errorf("request attempts exhausted")
}

func (c *Client) request(ctx context.Context, address string, gate *sourceGate) ([]byte, bool, error) {
	release, err := gate.acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	defer release()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/json")
	if req.URL.Host == "api.asmr.one" {
		req.AddCookie(
			&http.Cookie{
				Name:     "locale",
				Value:    req.URL.Query().Get("locale"),
				Secure:   true,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			},
		)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, ctx.Err() == nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests ||
		resp.Header.Get("Cf-Mitigated") != "" {
		gate.coolDown(resp.Header.Get("Retry-After"))
		return nil, false, fmt.Errorf("source blocked request: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode >= 500, &httpError{code: resp.StatusCode}
	}
	const maxBody = 16 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, true, err
	}
	if len(body) > maxBody {
		return nil, false, fmt.Errorf("upstream response exceeds size limit")
	}
	return body, false, nil
}

type httpError struct{ code int }

func (e *httpError) Error() string { return fmt.Sprintf("upstream HTTP %d", e.code) }
