package coverhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/coverhttp"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

// A loopback proxy is accepted (the public-address dial check applies only to
// direct connections), the CDN host is reached through CONNECT, and a host
// outside the allowlist is refused before any proxy contact.
func TestProxiedDownloadKeepsAllowlistAndSkipsDialCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var hits atomic.Int32
	var target atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		target.Store(r.Method + " " + r.Host)
		cancel()
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	proxy, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := coverhttp.NewProxied(func(*url.URL) (*url.URL, error) { return proxy, nil })

	if _, err = client.Download(ctx, "https://example.com/a.webp"); err == nil ||
		coverdelivery.Code(err) != "UNSUPPORTED_CDN" || hits.Load() != 0 {
		t.Fatalf("off-allowlist host: err=%v hits=%d", err, hits.Load())
	}
	_, _ = client.Download(ctx, "https://img.dlsite.jp/a.webp")
	if got, _ := target.Load().(string); got != "CONNECT img.dlsite.jp:443" {
		t.Fatalf("proxy saw %q", got)
	}
}

func TestProxiedDownloadFailsClosedWhenSettingIsUnreadable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	client := coverhttp.NewProxied(func(*url.URL) (*url.URL, error) { return nil, errors.New("setting unreadable") })
	if _, err := client.Download(ctx, "https://img.dlsite.jp/a.webp"); err == nil {
		t.Fatal("download succeeded with an unreadable proxy setting")
	}
}
