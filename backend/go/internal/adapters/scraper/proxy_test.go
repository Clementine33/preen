package scraper_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/scraper"
)

// connectProxy refuses every CONNECT with 502 and records its target, then
// cancels the caller's context so the client's retries stop after one attempt.
func connectProxy(t *testing.T, cancel context.CancelFunc) (*url.URL, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var targets []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		targets = append(targets, r.Method+" "+r.Host)
		mu.Unlock()
		cancel()
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), targets...) }
}

func TestProxiedRequestsFollowTheCurrentProxyAndFailClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	proxyA, hitsA := connectProxy(t, cancel)

	current := proxyA
	var failure error
	client := scraper.NewProxied(func(*url.URL) (*url.URL, error) { return current, failure })

	if _, err := client.FetchStatic(ctx, "RJ00000001", "ja_JP"); err == nil {
		t.Fatal("refused CONNECT succeeded")
	}
	if got := hitsA(); len(got) == 0 || got[0] != "CONNECT www.dlsite.com:443" {
		t.Fatalf("proxy A saw %v", got)
	}

	// A new value applies to the next request without rebuilding the client.
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	proxyB, hitsB := connectProxy(t, cancel)
	current = proxyB
	_, _ = client.FetchStatic(ctx, "RJ00000001", "ja_JP")
	if len(hitsB()) == 0 {
		t.Fatal("next request ignored the new proxy")
	}

	// An unreadable setting fails the request and never connects directly.
	ctx, cancel = context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	hitsBefore := len(hitsB())
	failure = errors.New("setting unreadable")
	if _, err := client.FetchStatic(ctx, "RJ00000001", "ja_JP"); err == nil {
		t.Fatal("request succeeded with an unreadable proxy setting")
	}
	if len(hitsB()) != hitsBefore {
		t.Fatal("request reached a proxy despite the error")
	}
}
