package httpapi //nolint:testpackage // white-box tests drive the routes through the real router

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"

	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
)

// fileFixture is one work bound to a real member directory whose files are
// real bytes on disk, with the inventory rows the manifest serves from.
type fileFixture struct {
	root   string
	member string
	png    []byte
	sjis   []byte
}

func seedWorkFiles(t *testing.T, fixture *sqlite.Repository) *fileFixture {
	t.Helper()
	root := t.TempDir()
	member := filepath.Join(root, "RJ01000001")
	if err := os.MkdirAll(filepath.Join(member, "表紙"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A real PNG whose magic bytes name their own type.
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	pngBytes := pngBuf.Bytes()
	if err := os.WriteFile(filepath.Join(member, "表紙", "cover_main.jpg"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	// Real Shift-JIS text: the sniff has to name it, the body must stay
	// exactly as stored.
	sjisBytes, _, err := transform.Bytes(japanese.ShiftJIS.NewEncoder(), []byte("【scene】こんにちは、声優さん。"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(member, "はじめにお読みください.txt"), sjisBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	// On disk but never scanned: the whitelist must refuse it.
	if err := os.WriteFile(filepath.Join(member, "unlisted.bin"), []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	memberPosix := filepath.ToSlash(member)
	if _, err := fixture.DB().Exec(
		"INSERT INTO libraries (id, name, root_path, root_path_key) VALUES ('lib-1', 'Onsei', ?, ?)",
		root, root); err != nil {
		t.Fatalf("seed library: %v", err)
	}
	if _, err := fixture.DB().Exec(`
		INSERT INTO works (id, title, collected_at, created_at, updated_at)
		VALUES ('RJ01000001', '雨宿り', ?, ?, ?)
	`, stamp, stamp, stamp); err != nil {
		t.Fatalf("seed work: %v", err)
	}
	if _, err := fixture.DB().Exec(`
		INSERT INTO work_copies (id, work_id, library_id, rel_path, created_at)
		VALUES ('c-1', 'RJ01000001', 'lib-1', 'RJ01000001', ?)
	`, stamp); err != nil {
		t.Fatalf("seed copy: %v", err)
	}

	mtime := time.Now().Unix()
	seedEntry := func(path, name, format string, size int64, isDir int) {
		t.Helper()
		if _, err := fixture.DB().Exec(`
			INSERT INTO entries (path, root_path, parent_path, name, is_dir, size, mtime, format)
			VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))
		`, path, filepath.ToSlash(root), filepath.ToSlash(filepath.Dir(path)), name, isDir, size, mtime, format); err != nil {
			t.Fatalf("seed entry %s: %v", path, err)
		}
	}
	seedEntry(memberPosix, "RJ01000001", "", 0, 1)
	seedEntry(memberPosix+"/表紙", "表紙", "", 0, 1)
	seedEntry(memberPosix+"/表紙/cover_main.jpg", "cover_main.jpg", "image/jpeg", int64(len(pngBytes)), 0)
	seedEntry(memberPosix+"/はじめにお読みください.txt", "はじめにお読みください.txt", "text/plain", int64(len(sjisBytes)), 0)

	return &fileFixture{root: root, member: memberPosix, png: pngBytes, sjis: sjisBytes}
}

// fileURL escapes each segment of the manifest path, the way the frontend
// builds it.
func fileURL(segments ...string) string {
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = url.PathEscape(segment)
	}
	return "/api/v1/works/RJ01000001/content/" + strings.Join(escaped, "/")
}

func TestWorkFileServesTheBytesItStores(t *testing.T) {
	var files *fileFixture
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		files = seedWorkFiles(t, fixture)
	})

	// The image's type comes from its content, not its lying extension.
	w := doRequest(t, engine, http.MethodGet, fileURL("c-1", "表紙", "cover_main.jpg"), nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("content type = %q, want image/png", ct)
	}
	if !bytes.Equal(w.Body.Bytes(), files.png) {
		t.Error("the body must be the stored bytes")
	}
	if ar := w.Header().Get("Accept-Ranges"); ar != "bytes" {
		t.Errorf("accept-ranges = %q", ar)
	}

	// The Shift-JIS text declares its charset and keeps its bytes.
	w = doRequest(t, engine, http.MethodGet, fileURL("c-1", "はじめにお読みください.txt"), nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=shift_jis" {
		t.Errorf("content type = %q, want shift_jis", ct)
	}
	if !bytes.Equal(w.Body.Bytes(), files.sjis) {
		t.Error("text bytes must never be re-encoded on the way out")
	}

	// An unlisted file is not served, and neither is a traversal.
	w = doRequest(t, engine, http.MethodGet, fileURL("c-1", "unlisted.bin"), nil, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("unlisted status = %d, want 404", w.Code)
	}
	w = doRequest(t, engine, http.MethodGet,
		"/api/v1/works/RJ01000001/content/c-1/"+url.PathEscape("../outside.txt"), nil, nil)
	if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
		t.Errorf("traversal status = %d, want 404 or 400", w.Code)
	}
}

func TestWorkFileRangeAndRevalidation(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedWorkFiles(t, fixture)
	})
	target := fileURL("c-1", "表紙", "cover_main.jpg")

	w := doRequest(t, engine, http.MethodGet, target, nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("the file must carry an etag")
	}

	// A conditional request answers 304 and no body.
	w = doRequest(t, engine, http.MethodGet, target, nil, map[string]string{"If-None-Match": etag})
	if w.Code != http.StatusNotModified {
		t.Errorf("revalidation status = %d, want 304", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("304 answers no body, got %d bytes", w.Body.Len())
	}

	// A single range answers 206 with exactly those bytes.
	w = doRequest(t, engine, http.MethodGet, target, nil, map[string]string{"Range": "bytes=0-3"})
	if w.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d (body=%s)", w.Code, w.Body.String())
	}
	if w.Body.Len() != 4 {
		t.Errorf("range answered %d bytes, want 4", w.Body.Len())
	}
}
