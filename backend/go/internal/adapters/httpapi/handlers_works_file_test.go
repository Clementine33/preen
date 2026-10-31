package httpapi //nolint:testpackage // white-box tests drive the routes through the real router

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
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
	// FLAC is outside Go's sniffer; its type must not depend on the host MIME database.
	flacBytes := append([]byte("fLaC"), make([]byte, 60)...)
	if err := os.WriteFile(filepath.Join(member, "01.flac"), flacBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	// An ID3v2 tag (600 bytes, past the sniff sample) in front of a FLAC stream.
	id3Bytes := append([]byte{'I', 'D', '3', 4, 0, 0, 0, 0, 4, 88}, make([]byte, 600)...)
	id3Bytes = append(append(id3Bytes, "fLaC"...), make([]byte, 60)...)
	if err := os.WriteFile(filepath.Join(member, "02.flac"), id3Bytes, 0o644); err != nil {
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
		INSERT INTO works (id, title, collected_at, updated_at)
		VALUES ('RJ01000001', '雨宿り', ?, ?)
	`, stamp, stamp); err != nil {
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
		inventoryfixture.SeedInventory(t, fixture, inventoryfixture.Row{
			Path:     path,
			RootPath: filepath.ToSlash(root),
			IsDir:    isDir == 1,
			Size:     size,
			Mtime:    mtime,
			Format:   format,
		})
	}
	seedEntry(memberPosix, "RJ01000001", "", 0, 1)
	seedEntry(memberPosix+"/表紙", "表紙", "", 0, 1)
	seedEntry(memberPosix+"/表紙/cover_main.jpg", "cover_main.jpg", "image/jpeg", int64(len(pngBytes)), 0)
	seedEntry(memberPosix+"/01.flac", "01.flac", "audio/flac", int64(len(flacBytes)), 0)
	seedEntry(memberPosix+"/02.flac", "02.flac", "audio/flac", int64(len(id3Bytes)), 0)
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

	// A host that registers the legacy alias must not change the answer.
	if err := mime.AddExtensionType(".flac", "audio/x-flac"); err != nil {
		t.Fatal(err)
	}
	w = doRequest(t, engine, http.MethodGet, fileURL("c-1", "01.flac"), nil, nil)
	if ct := w.Header().Get("Content-Type"); w.Code != http.StatusOK || ct != "audio/flac" {
		t.Errorf("flac response = %d %q, want audio/flac", w.Code, ct)
	}
	w = doRequest(t, engine, http.MethodGet, fileURL("c-1", "02.flac"), nil, nil)
	if ct := w.Header().Get("Content-Type"); w.Code != http.StatusOK || ct != "audio/flac" {
		t.Errorf("id3-tagged flac response = %d %q, want audio/flac", w.Code, ct)
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

func TestWorkTimedLyricsAreListedAndServed(t *testing.T) {
	for _, ext := range []string{"vtt", "VTT"} {
		t.Run(ext, func(t *testing.T) {
			name := "01_導入." + ext
			text := "WEBVTT\n\n00:01.000 --> 00:03.000\nこんにちは\n"
			engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
				wireWorks(d, fixture)
				files := seedWorkFiles(t, fixture)
				if err := os.WriteFile(filepath.Join(files.member, name), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
				inventoryfixture.SeedInventory(t, fixture, inventoryfixture.Row{
					Path:     files.member + "/" + name,
					RootPath: filepath.ToSlash(files.root),
					Size:     int64(len(text)),
					Mtime:    time.Now().Unix(),
				})
			})
			manifest := doRequest(t, engine, http.MethodGet, "/api/v1/works/RJ01000001/content", nil, nil)
			var body contentResponse
			if err := json.Unmarshal(manifest.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, node := range body.Copies[0].Tree.Children {
				if node.Name == name && node.Kind == "lrc" {
					found = true
				}
			}
			if !found {
				t.Fatal("timed lyrics missing from manifest")
			}
			w := doRequest(t, engine, http.MethodGet, fileURL("c-1", name), nil, nil)
			if w.Code != http.StatusOK || w.Body.String() != text ||
				w.Header().Get("Content-Type") != "text/vtt; charset=utf-8" {
				t.Fatalf("lyrics response = %d %s %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
			}
		})
	}
}

func TestWorkVideoIsServedWithRange(t *testing.T) {
	// An mp4 ftyp box: the sniffer names it video/mp4 whatever the inventory recorded.
	data := append([]byte{0, 0, 0, 0x18}, []byte("ftypmp42\x00\x00\x00\x00mp42isom")...)
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		files := seedWorkFiles(t, fixture)
		if err := os.WriteFile(filepath.Join(files.member, "clip.mp4"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		inventoryfixture.SeedInventory(t, fixture, inventoryfixture.Row{
			Path:     files.member + "/clip.mp4",
			RootPath: filepath.ToSlash(files.root),
			Size:     int64(len(data)),
			Mtime:    time.Now().Unix(),
		})
	})
	w := doRequest(t, engine, http.MethodGet, fileURL("c-1", "clip.mp4"), nil, map[string]string{"Range": "bytes=0-3"})
	if w.Code != http.StatusPartialContent || !strings.HasPrefix(w.Header().Get("Content-Type"), "video/") {
		t.Fatalf("video response = %d %q", w.Code, w.Header().Get("Content-Type"))
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
	w = doRequest(t, engine, http.MethodGet, target, nil, map[string]string{"Range": "bytes=999999-"})
	if w.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("invalid range = %d, want 416", w.Code)
	}
	w = doRequest(t, engine, http.MethodGet, target, nil, map[string]string{"Range": "bytes=0-1,3-4"})
	if w.Code != http.StatusOK {
		t.Errorf("multi range = %d, want whole file", w.Code)
	}
}

func TestWorkFileRejectsDirectoriesLinksAndFakeImageTypes(t *testing.T) {
	var files *fileFixture
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		files = seedWorkFiles(t, fixture)
	})
	w := doRequest(t, engine, http.MethodGet, fileURL("c-1", "表紙"), nil, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("directory = %d, want 404", w.Code)
	}
	cover := filepath.Join(files.member, "表紙", "cover_main.jpg")
	if err := os.WriteFile(cover, []byte("<html>not an image</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	w = doRequest(t, engine, http.MethodGet, fileURL("c-1", "表紙", "cover_main.jpg"), nil, nil)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("fake image = %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	if err := os.Remove(cover); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(outside, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, cover); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	w = doRequest(t, engine, http.MethodGet, fileURL("c-1", "表紙", "cover_main.jpg"), nil, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("symlink = %d, want 404", w.Code)
	}
}

func TestTextCharsetDoesNotMistakeSampleBoundaryForInvalidBytes(t *testing.T) {
	sjis, _, err := transform.Bytes(japanese.ShiftJIS.NewEncoder(), []byte("こんにちは声"))
	if err != nil {
		t.Fatal(err)
	}
	gb, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), []byte("𠀀𠀀"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		charset  string
		bytes    []byte
		charSize int
	}{
		{"utf-8", []byte("日本語あ"), 3},
		{"shift_jis", sjis, 2},
		{"gb18030", gb, 4},
	} {
		for cut := 1; cut < tc.charSize; cut++ {
			t.Run(tc.charset+strings.Repeat("-", cut), func(t *testing.T) {
				var files *fileFixture
				engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
					wireWorks(d, fixture)
					files = seedWorkFiles(t, fixture)
				})
				prefix := tc.bytes[:len(tc.bytes)-tc.charSize]
				data := append(bytes.Clone(prefix), bytes.Repeat([]byte("A"), 64*1024-len(prefix)-cut)...)
				data = append(data, tc.bytes[len(prefix):]...)
				if err := os.WriteFile(filepath.Join(files.member, "はじめにお読みください.txt"), data, 0o644); err != nil {
					t.Fatal(err)
				}
				w := doRequest(t, engine, http.MethodGet, fileURL("c-1", "はじめにお読みください.txt"), nil, nil)
				if got := w.Header().Get("Content-Type"); got != "text/plain; charset="+tc.charset {
					t.Errorf("charset = %s, want %s", got, tc.charset)
				}
				if !bytes.Equal(w.Body.Bytes(), data) {
					t.Error("the endpoint changed the source bytes")
				}
			})
		}
	}
}
