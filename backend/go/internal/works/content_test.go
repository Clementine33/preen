package works_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/onsei/organizer/backend/internal/fileops"
	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/works"
)

func TestClassifyContentKind(t *testing.T) {
	cases := map[[2]string]works.ContentFileKind{
		{"track01.lrc", ""}:                    works.KindLyrics,
		{"はじめにお読みください.txt", ""}:                works.KindText,
		{"01_導入.mp3", "audio/mpeg"}:            works.KindAudio,
		{"cover.png", "image/png"}:             works.KindImage,
		{"bonus.zip", ""}:                      works.KindUnknown,
		{"no_ext", "application/octet-stream"}: works.KindUnknown,
	}
	for input, want := range cases {
		if got := classifyKindOf(t, input); got != want {
			t.Errorf("%v: got %q, want %q", input, got, want)
		}
	}
}

// classifyKindOf routes the unexported classifier through the tree, which is
// the only shape it ships in.
func classifyKindOf(t *testing.T, input [2]string) works.ContentFileKind {
	t.Helper()
	name, format := input[0], input[1]
	parent := "/lib/RJ01000001"
	entries := []inventory.Entry{{
		Path: parent + "/" + name, ParentPath: parent, Name: name,
		Format: format,
	}}
	tree := works.BuildContentTree(parent, entries)
	if len(tree.Children) != 1 {
		t.Fatalf("tree of one entry has %d children", len(tree.Children))
	}
	return tree.Children[0].Kind
}

func TestBuildContentTree(t *testing.T) {
	member := "/lib/RJ01000001"
	entries := []inventory.Entry{
		{Path: member, ParentPath: "/lib", Name: "RJ01000001", IsDir: true},
		{Path: member + "/表紙", ParentPath: member, Name: "表紙", IsDir: true},
		{Path: member + "/表紙/cover_main.jpg", ParentPath: member + "/表紙", Name: "cover_main.jpg",
			Size: 4096, Format: "image/jpeg"},
		{Path: member + "/本編", ParentPath: member, Name: "本編", IsDir: true},
		{Path: member + "/本編/01_導入.mp3", ParentPath: member + "/本編", Name: "01_導入.mp3",
			Size: 5120, Format: "audio/mpeg"},
		{Path: member + "/本編/01_導入.lrc", ParentPath: member + "/本編", Name: "01_導入.lrc",
			Size: 1024},
		{Path: member + "/空", ParentPath: member, Name: "空", IsDir: true},
	}
	tree := works.BuildContentTree(member, entries)

	if tree.Name != "RJ01000001" || tree.RelPath != "" {
		t.Fatalf("root is the member directory, got %q rel %q", tree.Name, tree.RelPath)
	}
	// Directories before files, natural within each: kanji order is the
	// codepoint order the workbench's member tree also uses.
	names := make([]string, 0, len(tree.Children))
	for _, child := range tree.Children {
		names = append(names, child.Name)
	}
	if !reflect.DeepEqual(names, []string{"本編", "空", "表紙"}) {
		t.Fatalf("children order = %v", names)
	}
	if len(tree.Children[1].Children) != 0 || tree.Children[1].Children == nil {
		t.Errorf("empty directory must carry an empty children slice")
	}

	cover := tree.Children[2].Children[0]
	if cover.Kind != works.KindImage || cover.RelPath != "表紙/cover_main.jpg" || cover.Size != 4096 {
		t.Errorf("cover file wrong: %+v", cover)
	}
	if cover.Width != nil || cover.DurationSec != nil || cover.Format != nil {
		t.Errorf("image carries no audio facts or container, got %+v", cover)
	}

	track := tree.Children[0].Children[1]
	if track.Kind != works.KindAudio || track.Format == nil || *track.Format != "MP3" {
		t.Errorf("audio container wrong: %+v", track)
	}
	lyric := tree.Children[0].Children[0]
	if lyric.Kind != works.KindLyrics || lyric.Format != nil {
		t.Errorf("lyric wrong: %+v", lyric)
	}
}

func TestContentNodeJSONShapes(t *testing.T) {
	zero := int64(0)
	file, err := json.Marshal(works.ContentNode{Name: "a.lrc", RelPath: "a.lrc",
		Kind: works.KindLyrics, Size: 0})
	if err != nil {
		t.Fatal(err)
	}
	var fileFields map[string]any
	if unmarshalErr := json.Unmarshal(file, &fileFields); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	// A zero-size file still answers size: 0, and unmeasured facts answer
	// null, not a missing key.
	if fileFields["size"] != float64(zero) {
		t.Errorf("file size = %v, want 0", fileFields["size"])
	}
	for _, key := range []string{"format", "bitrate", "width", "height", "duration_sec"} {
		if _, present := fileFields[key]; !present {
			t.Errorf("file missing %q: %s", key, file)
		}
	}
	if _, present := fileFields["children"]; present {
		t.Errorf("file must not carry children: %s", file)
	}

	dir, err := json.Marshal(works.ContentNode{Name: "本編", RelPath: "本編"})
	if err != nil {
		t.Fatal(err)
	}
	if string(dir) != `{"name":"本編","rel_path":"本編","children":[]}` {
		t.Errorf("empty dir marshals as %s", dir)
	}
}

// contentStore answers only the manifest's read; the rest of the catalog is
// out of scope here and stays nil behind the interface.
type contentStore struct {
	works.Store

	roots []works.CopyRoot
	found bool
}

func (s *contentStore) WorkCopyRoots(id works.WorkCode) ([]works.CopyRoot, bool, error) {
	return s.roots, s.found && id == "RJ01000001", nil
}

type fakeEntries struct {
	entries []inventory.Entry
}

func (f *fakeEntries) ListEntriesUnderPath(_ string) ([]inventory.Entry, error) {
	return f.entries, nil
}

func (f *fakeEntries) EntryByPath(path string) (inventory.Entry, bool, error) {
	for _, entry := range f.entries {
		if entry.Path == path && !entry.IsDir {
			return entry, true, nil
		}
	}
	return inventory.Entry{}, false, nil
}

func TestContentManifest(t *testing.T) {
	offline := errors.New("member gone")
	service := works.NewService(
		&contentStore{roots: []works.CopyRoot{
			{CopyID: "c-1", LibraryID: "lib-1", RelPath: "RJ01000001", RootPath: "/lib"},
			{CopyID: "c-2", LibraryID: "lib-2", RelPath: "RJ01000002", RootPath: "/gone"},
		}, found: true},
		&fakeEntries{entries: []inventory.Entry{
			{Path: "/lib/RJ01000001", ParentPath: "/lib", Name: "RJ01000001", IsDir: true},
		}},
		func(rootPath, memberRel string) (string, error) {
			if rootPath == "/gone" {
				return "", offline
			}
			return rootPath + "/" + memberRel, nil
		},
		func(rootPath, memberRel, rel string) (string, error) {
			return rootPath + "/" + memberRel + "/" + rel, nil
		},
	)

	content, refusal := service.Content("RJ01000001")
	if refusal != nil {
		t.Fatalf("refusal: %v", refusal)
	}
	if content.RJ != "RJ01000001" || len(content.Copies) != 2 {
		t.Fatalf("manifest wrong: %+v", content)
	}
	online, missing := content.Copies[0], content.Copies[1]
	if !online.Online || online.Tree == nil {
		t.Errorf("resolvable member must be online with a tree: %+v", online)
	}
	if online.DirID == "" {
		t.Errorf("dir id must be resolved server-side")
	}
	if missing.Online || missing.Tree != nil {
		t.Errorf("gone member must answer offline without a tree: %+v", missing)
	}

	// An unknown work is a 404-class refusal, and a placeholder answers empty.
	if _, refusal = service.Content("RJ99999999"); refusal == nil {
		t.Fatal("expected a refusal for an unknown work")
	}
	placeholder := works.NewService(&contentStore{found: true}, &fakeEntries{},
		func(string, string) (string, error) { return "", nil },
		func(string, string, string) (string, error) { return "", nil })
	if content, refusal = placeholder.Content("RJ01000001"); refusal != nil || len(content.Copies) != 0 {
		t.Fatalf("placeholder answers empty copies, got %+v refusal %v", content, refusal)
	}
}

func TestContentFileLocatesManifestPaths(t *testing.T) {
	const rj = works.WorkCode("RJ01000001")
	roots := []works.CopyRoot{
		{CopyID: "c-1", LibraryID: "lib-1", RelPath: "RJ01000001", RootPath: "/lib"},
	}
	newService := func(entries *fakeEntries) *works.Service {
		return works.NewService(
			&contentStore{roots: roots, found: true},
			entries,
			func(rootPath, memberRel string) (string, error) {
				return rootPath + "/" + memberRel, nil
			},
			func(rootPath, memberRel, rel string) (string, error) {
				return rootPath + "/" + memberRel + "/" + rel, nil
			},
		)
	}

	// The manifest holds the cover; the file resolves and carries its facts.
	service := newService(&fakeEntries{entries: []inventory.Entry{
		{Path: "/lib/RJ01000001/表紙/cover_main.jpg", Name: "cover_main.jpg",
			Format: "image/jpeg", Size: 4096, Mtime: 1759000000, ContentRev: 3},
	}})
	loc, refusal := service.ContentFile(rj, "c-1", "表紙/cover_main.jpg")
	if refusal != nil {
		t.Fatalf("refusal: %v", refusal)
	}
	if loc.AbsPath != "/lib/RJ01000001/表紙/cover_main.jpg" || loc.Kind != works.KindImage {
		t.Errorf("location wrong: %+v", loc)
	}
	if loc.Size != 4096 || loc.Mtime != 1759000000 || loc.ContentRev != 3 {
		t.Errorf("etag facts wrong: %+v", loc)
	}

	// A path the inventory does not hold is not served, whatever the disk has.
	if _, refusal = service.ContentFile(rj, "c-1", "not-in-manifest.bin"); refusal == nil ||
		refusal.Code != works.CodeWorkNotFound {
		t.Fatalf("unlisted path must be refused, got %+v", refusal)
	}
	// Neither is another work's copy id, or an unknown work's.
	if _, refusal = service.ContentFile(rj, "c-other", "表紙/cover_main.jpg"); refusal == nil {
		t.Fatal("a foreign copy must be refused")
	}
	if _, refusal = service.ContentFile("RJ99999999", "c-1", "x"); refusal == nil {
		t.Fatal("an unknown work must be refused")
	}

	// A containment refusal crosses as the catalog's own refusal.
	rejecting := works.NewService(
		&contentStore{roots: roots, found: true},
		&fakeEntries{entries: []inventory.Entry{
			{Path: "/lib/RJ01000001/escape", Name: "escape"},
		}},
		func(rootPath, memberRel string) (string, error) { return rootPath + "/" + memberRel, nil },
		func(string, string, string) (string, error) {
			return "", &fileops.PathError{Code: fileops.CodeSymlink, Message: "the path goes through a symbolic link"}
		},
	)
	if _, refusal = rejecting.ContentFile(rj, "c-1", "escape"); refusal == nil ||
		refusal.Kind != works.KindNotFound {
		t.Fatalf("symlink must be refused as not-found, got %+v", refusal)
	}
}
