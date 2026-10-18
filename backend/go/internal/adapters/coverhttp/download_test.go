package coverhttp_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/coverhttp"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDownloadUsesContentFormatAndRejectsUntrustedSources(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	client := coverhttp.New(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials reached CDN")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"image/webp"}},
			Body:       io.NopCloser(bytes.NewReader(b.Bytes())),
		}, nil
	})})
	got, err := client.Download(
		context.Background(),
		"https://img.dlsite.jp/incorrect.webp",
		coverdelivery.Validators{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Extension != "png" || got.ContentType != "image/png" {
		t.Fatalf("format was guessed from URL/header: %+v", got)
	}
	if _, err = client.Download(t.Context(), "https://127.0.0.1/image", coverdelivery.Validators{}); err == nil {
		t.Fatal("untrusted source accepted")
	}
}

func TestDownloadRejectsCrossCDNRedirectAndInvalidBodies(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		header http.Header
		length int64
		body   string
		code   string
	}{
		{"redirect", 302, http.Header{"Location": {"https://evil.example/main.webp"}}, 0, "", "UNSUPPORTED_CDN"},
		{"html", 200, nil, 0, "<html>denied</html>", "INVALID_IMAGE"},
		{"large", 200, nil, coverdelivery.MaxBytes + 1, "", "IMAGE_TOO_LARGE"},
		{"unsolicited 304", 304, nil, 0, "", "DOWNLOAD_FAILED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				client := coverhttp.New(
					&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
						if r.URL.Host != "img.dlsite.jp" {
							t.Fatal("followed unsupported CDN")
						}
						return &http.Response{
							StatusCode:    tc.status,
							Header:        tc.header,
							ContentLength: tc.length,
							Body:          io.NopCloser(bytes.NewBufferString(tc.body)),
						}, nil
					})},
				)
				if _, err := client.Download(
					t.Context(),
					"https://img.dlsite.jp/main.webp",
					coverdelivery.Validators{},
				); coverdelivery.Code(
					err,
				) != tc.code {
					t.Fatalf("failure: %v", err)
				}
			})
		})
	}
}
func TestDownloadBackoffIsBoundedAndHonorsCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		client := coverhttp.New(&http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Retry-After": {"2"}},
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		})})
		at := time.Now()
		if _, err := client.Download(
			t.Context(),
			"https://img.dlsite.jp/main.webp",
			coverdelivery.Validators{},
		); err == nil {
			t.Fatal("429 accepted")
		}
		if calls != 3 || time.Since(at) != 4*time.Second {
			t.Fatalf("retry bound: %d %v", calls, time.Since(at))
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := client.Download(ctx, "https://img.dlsite.jp/main.webp", coverdelivery.Validators{}); err == nil {
			t.Fatal("cancellation ignored")
		}
	})
}
func TestDownloadConditionalsAreIndependentOfCredentials(t *testing.T) {
	client := coverhttp.New(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("If-None-Match") != "saved" || r.Header.Get("Authorization") != "" ||
			r.Header.Get("Cookie") != "" {
			t.Fatal("conditional credentials incorrect")
		}
		return &http.Response{StatusCode: http.StatusNotModified, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})})
	result, err := client.Download(
		t.Context(),
		"https://img.dlsite.jp/main.webp",
		coverdelivery.Validators{ETag: "saved"},
	)
	if err != nil || !result.NotModified {
		t.Fatalf("304: %+v %v", result, err)
	}
}
