package httpapi //nolint:testpackage // white-box tests drive the routes through the real router

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/media"
	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/thumbnail"
	"github.com/onsei/organizer/backend/internal/works"
)

func thumbURL(query string, segments ...string) string {
	return strings.Replace(fileURL(segments...), "/content/", "/thumbs/", 1) + "?" + query
}

type failedThumbEngine struct{ err error }

func (e failedThumbEngine) Thumbnail(string, int) (thumbnail.Image, error) {
	return thumbnail.Image{}, e.err
}

func TestWorkThumbErrorsNeverCacheTheSuccessfulVersion(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"busy", thumbnail.ErrBusy, http.StatusServiceUnavailable},
		{"too large", thumbnail.ErrTooLarge, http.StatusUnprocessableEntity},
		{"undecodable", thumbnail.ErrUndecodable, http.StatusUnprocessableEntity},
		{"io failure", errors.New("read failed"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var version string
			engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
				wireWorks(d, fixture)
				seedWorkFiles(t, fixture)
				d.Thumbs = thumbnail.New(failedThumbEngine{tc.err}, t.TempDir(), 1<<20, 1, 1)
				content, refusal := d.Works.Content(works.WorkCode("RJ01000001"))
				if refusal != nil {
					t.Fatal(refusal)
				}
				version = content.Copies[0].Tree.Children[0].Children[0].ThumbV
			})
			w := doRequest(t, engine, http.MethodGet,
				thumbURL("h=120&v="+version, "c-1", "表紙", "cover_main.jpg"), nil, nil)
			if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
				t.Errorf("error response = %d %v", w.Code, w.Header())
			}
			if tc.status == http.StatusServiceUnavailable && w.Header().Get("Retry-After") == "" {
				t.Error("busy response lost Retry-After")
			}
		})
	}
}

func TestWorkThumbIsImmutableOnlyAtItsVersion(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		d.Thumbs = thumbnail.New(media.ThumbEngine{}, t.TempDir(), 1<<20, 1, 1)
		seedWorkFiles(t, fixture)
	})

	// A stale or missing version still answers, but never as immutable.
	w := doRequest(t, engine, http.MethodGet, thumbURL("h=100", "c-1", "表紙", "cover_main.jpg"), nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("stale version cache-control = %q", cc)
	}
	// The fixture's transparent PNG stays PNG.
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("content type = %q", ct)
	}
	etag := w.Header().Get("ETag")
	version := strings.TrimSuffix(strings.TrimPrefix(etag, `"`), `-120"`)
	if version == etag || version == "" {
		t.Fatalf("etag %q does not carry the version at the snapped height", etag)
	}

	w = doRequest(t, engine, http.MethodGet,
		thumbURL("h=100&v="+version, "c-1", "表紙", "cover_main.jpg"), nil, nil)
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("current version cache-control = %q", cc)
	}
	w = doRequest(t, engine, http.MethodGet, thumbURL("h=100&v="+version, "c-1", "表紙", "cover_main.jpg"),
		nil, map[string]string{"If-None-Match": etag})
	if w.Code != http.StatusNotModified {
		t.Errorf("revalidation status = %d", w.Code)
	}

	// Only manifest images, and only at a real height.
	w = doRequest(t, engine, http.MethodGet, thumbURL("h=100", "c-1", "はじめにお読みください.txt"), nil, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("text thumbnail status = %d, want 404", w.Code)
	}
	w = doRequest(t, engine, http.MethodGet, thumbURL("h=0", "c-1", "表紙", "cover_main.jpg"), nil, nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("zero height status = %d, want 400", w.Code)
	}
}
