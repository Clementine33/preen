package httpapi //nolint:testpackage // white-box tests drive the routes through the real router

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/library"
)

// contentResponse mirrors the manifest the works types marshal.
type contentResponse struct {
	RJ     string `json:"rj"`
	Copies []struct {
		CopyID  string              `json:"copy_id"`
		Library string              `json:"library_id"`
		DirID   string              `json:"dir_id"`
		Online  bool                `json:"online"`
		Tree    contentTreeResponse `json:"tree"`
	} `json:"copies"`
}

type contentTreeResponse struct {
	Name     string                `json:"name"`
	RelPath  string                `json:"rel_path"`
	Children []contentNodeResponse `json:"children"`
}

type contentNodeResponse struct {
	Name     string                `json:"name"`
	RelPath  string                `json:"rel_path"`
	Children []contentNodeResponse `json:"children"`
	Kind     string                `json:"kind"`
	Size     int64                 `json:"size"`
}

// seedContentWork binds one work to a real member directory under a real
// library root, and stocks the inventory the manifest reads: one dir, one
// cover image, one track.
func seedContentWork(t *testing.T, fixture *sqlite.Repository) string {
	t.Helper()
	root := t.TempDir()
	member := filepath.Join(root, "RJ01000001")
	if err := os.MkdirAll(filepath.Join(member, "表紙"), 0o755); err != nil {
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
	seedEntry := func(path string, name string, isDir int, size int64, format string) {
		t.Helper()
		if _, err := fixture.DB().Exec(`
			INSERT INTO entries (path, root_path, parent_path, name, is_dir, size, mtime, format)
			VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))
		`, path, filepath.ToSlash(root), filepath.ToSlash(filepath.Dir(path)), name, isDir, size, mtime, format); err != nil {
			t.Fatalf("seed entry %s: %v", path, err)
		}
	}
	seedEntry(memberPosix, "RJ01000001", 1, 0, "")
	seedEntry(memberPosix+"/表紙", "表紙", 1, 0, "")
	seedEntry(memberPosix+"/表紙/cover_main.jpg", "cover_main.jpg", 0, 4096, "image/jpeg")
	seedEntry(memberPosix+"/01_導入.mp3", "01_導入.mp3", 0, 5120, "audio/mpeg")
	return root
}

func TestGetWorkContentAnswersTheBoundCopysTree(t *testing.T) {
	var root string
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		root = seedContentWork(t, fixture)
	})

	w := doRequest(t, engine, http.MethodGet, "/api/v1/works/RJ01000001/content", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}
	var body contentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.RJ != "RJ01000001" || len(body.Copies) != 1 {
		t.Fatalf("manifest wrong: %+v", body)
	}
	cp := body.Copies[0]
	if cp.CopyID != "c-1" || !cp.Online {
		t.Errorf("copy wrong: %+v", cp)
	}
	if want := library.DirID("lib-1", root, "RJ01000001"); cp.DirID != want {
		t.Errorf("dir_id = %s, want %s", cp.DirID, want)
	}
	// The tree is the stored inventory of the member: the cover set and the
	// root-level track, classified by kind.
	if len(cp.Tree.Children) != 2 {
		t.Fatalf("member children = %+v", cp.Tree.Children)
	}
	if cp.Tree.Children[0].Kind != "" || len(cp.Tree.Children[0].Children) != 1 {
		t.Fatalf("cover dir first with one child, got %+v", cp.Tree.Children)
	}
	if cp.Tree.Children[0].Children[0].Kind != "img" || cp.Tree.Children[0].Children[0].Size != 4096 {
		t.Errorf("cover file wrong: %+v", cp.Tree.Children[0].Children[0])
	}
	if cp.Tree.Children[1].Kind != "aud" || cp.Tree.Children[1].RelPath != "01_導入.mp3" {
		t.Errorf("track wrong: %+v", cp.Tree.Children[1])
	}
}

func TestGetWorkContentOfBoundlessAndUnknownWorks(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedHTTPWork(t, fixture, "RJ01000002", "関係だけ")
	})

	// A collected work without copies has a catalog entry and no copies: an empty manifest,
	// not a 404.
	w := doRequest(t, engine, http.MethodGet, "/api/v1/works/RJ01000002/content", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("copyless status = %d (body=%s)", w.Code, w.Body.String())
	}
	var body contentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Copies) != 0 {
		t.Errorf("copyless copies = %+v", body.Copies)
	}

	w = doRequest(t, engine, http.MethodGet, "/api/v1/works/RJ99999999/content", nil, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown work status = %d, want 404", w.Code)
	}
}
