package githubrelease_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/githubrelease"
	"github.com/Clementine33/preen/backend/internal/updatecheck"
)

// serve answers every request with the next scripted response, repeating the
// last one, and counts the requests.
func serve(t *testing.T, responses ...func(http.ResponseWriter)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1))
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("User-Agent") != "preen/0.2.0" {
			t.Errorf("headers: Accept=%q User-Agent=%q", r.Header.Get("Accept"), r.Header.Get("User-Agent"))
		}
		responses[min(n, len(responses))-1](w)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func status(code int) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { w.WriteHeader(code) }
}

func body(s string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { _, _ = w.Write([]byte(s)) }
}

func noSleep(context.Context, time.Duration) error { return nil }

// check runs the update check over the real client against srv.
func check(srv *httptest.Server, current string) *updatecheck.Service {
	client := githubrelease.New(srv.Client(), srv.URL, "0.2.0")
	svc := updatecheck.New(current, client.Latest, updatecheck.WithSleep(noSleep))
	svc.Run(context.Background())
	return svc
}

const latest = `{"tag_name":"v0.3.0","html_url":"https://github.com/Clementine33/preen/releases/tag/v0.3.0","body":"notes"}`

func TestNewerReleaseIsAvailable(t *testing.T) {
	srv, calls := serve(t, body(latest))
	got := check(srv, "0.2.0").Status()
	want := updatecheck.Status{
		State: updatecheck.StateAvailable, LatestVersion: "v0.3.0",
		ReleaseURL: "https://github.com/Clementine33/preen/releases/tag/v0.3.0",
	}
	if got != want || calls.Load() != 1 {
		t.Fatalf("status = %+v after %d requests", got, calls.Load())
	}
}

func TestUpToDateAndOlderReleasesReportNone(t *testing.T) {
	for _, current := range []string{"0.3.0", "0.4.0"} {
		srv, _ := serve(t, body(latest))
		if got := check(srv, current).Status().State; got != updatecheck.StateNone {
			t.Errorf("current %s: state = %q", current, got)
		}
	}
}

func TestReleaseCandidateSeesTheStableRelease(t *testing.T) {
	srv, _ := serve(t, body(`{"tag_name":"v0.2.0","html_url":"https://example.test/r"}`))
	if got := check(srv, "0.2.0-rc.2").Status().State; got != updatecheck.StateAvailable {
		t.Fatalf("state = %q", got)
	}
}

func TestServerErrorsAndRateLimitsRetryUpToThreeAttempts(t *testing.T) {
	for _, code := range []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusTooManyRequests} {
		srv, calls := serve(t, status(code))
		if got := check(srv, "0.2.0").Status().State; got != updatecheck.StateNone || calls.Load() != 3 {
			t.Errorf("status %d: state = %q after %d requests", code, got, calls.Load())
		}
	}
	srv, calls := serve(t, status(http.StatusServiceUnavailable), status(http.StatusTooManyRequests), body(latest))
	if got := check(srv, "0.2.0").Status().State; got != updatecheck.StateAvailable || calls.Load() != 3 {
		t.Fatalf("recovery: state = %q after %d requests", got, calls.Load())
	}
}

func TestNetworkErrorsRetryUpToThreeAttempts(t *testing.T) {
	srv, calls := serve(t, body(latest))
	endpoint := srv.URL
	srv.Close()
	client := githubrelease.New(nil, endpoint, "0.2.0")
	var attempts atomic.Int32
	svc := updatecheck.New("0.2.0", func(ctx context.Context) (updatecheck.Release, error) {
		attempts.Add(1)
		return client.Latest(ctx)
	}, updatecheck.WithSleep(noSleep))
	svc.Run(context.Background())
	if attempts.Load() != 3 || calls.Load() != 0 || svc.Status().State != updatecheck.StateNone {
		t.Fatalf("attempts = %d, served = %d, state = %q", attempts.Load(), calls.Load(), svc.Status().State)
	}
}

func TestClientErrorsAndUnreadableBodiesAreNotRetried(t *testing.T) {
	for name, respond := range map[string]func(http.ResponseWriter){
		"404":        status(http.StatusNotFound),
		"403":        status(http.StatusForbidden),
		"bad json":   body("<html>"),
		"no tag":     body(`{"html_url":"https://example.test/r"}`),
		"bad semver": body(`{"tag_name":"nightly","html_url":"https://example.test/r"}`),
	} {
		srv, calls := serve(t, respond)
		if got := check(srv, "0.2.0").Status().State; got != updatecheck.StateNone || calls.Load() != 1 {
			t.Errorf("%s: state = %q after %d requests", name, got, calls.Load())
		}
	}
}

func TestCancelledContextStopsTheCheck(t *testing.T) {
	srv, calls := serve(t, status(http.StatusServiceUnavailable))
	ctx, cancel := context.WithCancel(context.Background())
	client := githubrelease.New(srv.Client(), srv.URL, "0.2.0")
	svc := updatecheck.New(
		"0.2.0",
		client.Latest,
		updatecheck.WithSleep(func(ctx context.Context, _ time.Duration) error {
			cancel()
			return ctx.Err()
		}),
	)
	svc.Run(ctx)
	if calls.Load() != 1 || svc.Status().State != updatecheck.StateChecking {
		t.Fatalf("after cancel: %d requests, state = %q", calls.Load(), svc.Status().State)
	}
}

// TestRequestsFollowTheOutboundProxy sends the production request to the proxy
// the resolver names, and fails it without connecting when the resolver errors.
func TestRequestsFollowTheOutboundProxy(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.Host)
		mu.Unlock()
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}

	var targets []string
	var failure error
	client := githubrelease.NewProxied(func(target *url.URL) (*url.URL, error) {
		targets = append(targets, target.String())
		return proxyURL, failure
	}, "0.2.0")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err = client.Latest(ctx); err == nil {
		t.Fatal("refused CONNECT succeeded")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "CONNECT api.github.com:443" {
		t.Fatalf("proxy saw %v", seen)
	}
	if len(targets) != 1 || targets[0] != githubrelease.LatestURL {
		t.Fatalf("resolver asked for %v", targets)
	}

	// An unreadable setting fails the request and never connects directly.
	failure = errors.New("setting unreadable")
	if _, err = client.Latest(ctx); err == nil || len(seen) != 1 {
		t.Fatalf("an unreadable setting did not fail the request: %v, proxy saw %v", err, seen)
	}
}
