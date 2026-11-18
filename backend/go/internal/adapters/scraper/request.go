package scraper

import (
	"context"
	"encoding/json"
	"errors"
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
// A 429 opens a throttle window that requests wait out; challenges and an
// unreachable network open a fail-fast window. Each deadline only extends.
type sourceGate struct {
	slots                               chan struct{}
	interval                            time.Duration
	mu                                  sync.Mutex
	next, throttledUntil, failFastUntil time.Time
	transportFails                      int
	// tripped is closed and replaced whenever failFastUntil extends, waking
	// requests that are waiting out a throttle window.
	tripped chan struct{}
}

// transportTrip is how many consecutive transport failures (no HTTP response)
// put a source into cooldown, so an unreachable network ends a batch quickly.
const transportTrip = 5

// throttleBudget bounds how long one logical GET waits out throttle windows.
const throttleBudget = 10 * time.Minute

// errThrottled marks an HTTP 429 that put the source into a throttle window.
var errThrottled = errors.New("source rate limited request: HTTP 429")

func newSourceGate(concurrency int, interval time.Duration) *sourceGate {
	return &sourceGate{slots: make(chan struct{}, concurrency), interval: interval, tripped: make(chan struct{})}
}

// acquire takes a slot and a start time, waiting out throttle windows while
// *budget lasts; a throttle wait releases the slot and re-queues afterwards.
func (g *sourceGate) acquire(ctx context.Context, budget *time.Duration) (func(), error) {
	for {
		select {
		case g.slots <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		release := func() { <-g.slots }
		g.mu.Lock()
		wait, err := g.cooling(*budget)
		tripped := g.tripped
		start := time.Now()
		if err == nil && wait == 0 {
			if g.next.After(start) {
				start = g.next
			}
			g.next = start.Add(g.interval)
		}
		g.mu.Unlock()
		if err == nil && wait == 0 {
			if err = pause(ctx, time.Until(start)); err == nil {
				g.mu.Lock()
				wait, err = g.cooling(*budget)
				tripped = g.tripped
				g.mu.Unlock()
				if err == nil && wait == 0 {
					return release, nil
				}
			}
		}
		release()
		if err != nil {
			return nil, err
		}
		began := time.Now()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-tripped:
			timer.Stop()
		case <-timer.C:
		}
		*budget -= time.Since(began)
	}
}

// cooling reports how long to wait out a throttle window, or why not to wait.
// The caller holds g.mu.
func (g *sourceGate) cooling(budget time.Duration) (time.Duration, error) {
	now := time.Now()
	if now.Before(g.failFastUntil) {
		return 0, fmt.Errorf("source is cooling down (%s left)", g.failFastUntil.Sub(now).Round(time.Second))
	}
	wait := g.throttledUntil.Sub(now)
	if wait > budget {
		return 0, fmt.Errorf("source is rate limited (%s left)", wait.Round(time.Second))
	}
	return max(wait, 0), nil
}

// cooldownUntil honors Retry-After seconds or date, with a 60-second minimum.
func cooldownUntil(header string) time.Time {
	until := time.Now().Add(time.Minute)
	if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
		until = until.Add(time.Duration(seconds)*time.Second - time.Minute)
	} else if date, err := http.ParseTime(header); err == nil && date.After(until) {
		until = date
	}
	if minimum := time.Now().Add(time.Minute); minimum.After(until) {
		until = minimum
	}
	return until
}

// throttle extends the throttle window and reports the time left in it.
func (g *sourceGate) throttle(until time.Time) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	if until.After(g.throttledUntil) {
		g.throttledUntil = until
	}
	return time.Until(g.throttledUntil)
}

// failFast extends the fail-fast window and wakes throttle waiters. The caller holds g.mu.
func (g *sourceGate) failFast(until time.Time) {
	if until.After(g.failFastUntil) {
		g.failFastUntil = until
		close(g.tripped)
		g.tripped = make(chan struct{})
	}
}

// transport records the outcome of one request: any HTTP response proves the
// network works; consecutive failures to get one cool the source down.
func (g *sourceGate) transport(failed bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !failed {
		g.transportFails = 0
		return
	}
	if g.transportFails++; g.transportFails >= transportTrip {
		// One more failure after the cooldown trips it again.
		g.transportFails = transportTrip - 1
		g.failFast(time.Now().Add(time.Minute))
	}
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
	budget, throttled := throttleBudget, false
	for attempt := range 3 {
		body, retry, requestErr := c.request(ctx, address, gate, &budget)
		if requestErr == nil {
			return json.Unmarshal(body, out)
		}
		if errors.Is(requestErr, errThrottled) && !throttled && attempt < 2 {
			// The next acquire waits out the throttle window; resend once.
			throttled = true
			continue
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

func (c *Client) request(
	ctx context.Context,
	address string,
	gate *sourceGate,
	budget *time.Duration,
) ([]byte, bool, error) {
	release, err := gate.acquire(ctx, budget)
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
		if ctx.Err() == nil {
			gate.transport(true)
		}
		return nil, ctx.Err() == nil, err
	}
	defer resp.Body.Close()
	gate.transport(false)
	if resp.StatusCode == http.StatusForbidden || resp.Header.Get("Cf-Mitigated") != "" {
		gate.mu.Lock()
		gate.failFast(cooldownUntil(resp.Header.Get("Retry-After")))
		gate.mu.Unlock()
		return nil, false, fmt.Errorf("source blocked request: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		left := gate.throttle(cooldownUntil(resp.Header.Get("Retry-After")))
		return nil, false, fmt.Errorf("%w (%s left)", errThrottled, left.Round(time.Second))
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
