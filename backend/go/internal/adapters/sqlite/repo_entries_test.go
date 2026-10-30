package sqlite_test

import (
	"path/filepath"
	"reflect"
	"strings"

	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/works"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
)

// TestObservedAudioEntriesSpansTheWholeSubtree pins the path boundaries a
// planning root is collected with: the root directory itself ("/", kept
// defensively even though no member folder can be it), a root spelled with a
// trailing slash, and a root whose audio sits several directories down. The
// range predicate is a binary one over the path index, so its edges are the
// contract — a sibling whose name merely starts with the root's name is a
// different subtree.
func TestObservedAudioEntriesSpansTheWholeSubtree(t *testing.T) {
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	now := time.Now().Unix()
	for _, path := range []string{
		"/music/a/01.mp3",
		"/music/a/deep/02.mp3",
		"/music/ab/03.mp3",
		"/other/04.mp3",
	} {
		inventoryfixture.SeedInventory(
			t,
			repo,
			inventoryfixture.Row{
				Path:      path,
				RootPath:  filepath.Dir(path),
				Format:    "mp3",
				CreatedAt: now,
			},
		)
	}
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{Path: "/music/a/deep", RootPath: "/music/a", IsDir: true, CreatedAt: now},
	)

	for _, tc := range []struct {
		root string
		want []string
	}{
		{"/", []string{"/music/a/01.mp3", "/music/a/deep/02.mp3", "/music/ab/03.mp3", "/other/04.mp3"}},
		{"/music/", []string{"/music/a/01.mp3", "/music/a/deep/02.mp3", "/music/ab/03.mp3"}},
		{"/music/a", []string{"/music/a/01.mp3", "/music/a/deep/02.mp3"}},
		{"/music/a/", []string{"/music/a/01.mp3", "/music/a/deep/02.mp3"}},
		{"/music/ab", []string{"/music/ab/03.mp3"}},
	} {
		entries, err := repo.ObservedAudioEntries(tc.root)
		if err != nil {
			t.Fatalf("collect %s: %v", tc.root, err)
		}
		got := make([]string, 0, len(entries))
		for _, e := range entries {
			got = append(got, e.Path)
		}
		if len(got) != len(tc.want) {
			t.Errorf("root %q collected %v, want %v", tc.root, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("root %q collected %v, want %v", tc.root, got, tc.want)
				break
			}
		}
	}
}

func TestInventoryReadsPreservePathIdentityAndFileFacts(t *testing.T) {
	for _, root := range []string{"/", "C:/", "//server/share", "/music"} {
		t.Run(root, func(t *testing.T) { checkInventoryReadPaths(t, root) })
	}
}

func checkInventoryReadPaths(t *testing.T, root string) {
	t.Helper()
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	prefix := strings.TrimSuffix(root, "/") + "/"
	work := prefix + "RJ000001%_日"
	file := work + "/disc/track.mp3"
	inventoryfixture.SeedInventory(t, repo,
		inventoryfixture.Row{Path: work, RootPath: root, IsDir: true, IsWorkRoot: true, CreatedAt: 123, Mtime: 17},
		inventoryfixture.Row{Path: file, RootPath: root, Size: 11, Mtime: 22, ContentRev: 3, Format: "mp3",
			Bitrate: new(int32(320000)), Width: new(50), Height: new(60), DurationSec: new(7.5)},
		inventoryfixture.Row{Path: prefix + "direct.jpg", RootPath: root},
		inventoryfixture.Row{Path: work + " sibling/other.mp3", RootPath: root},
	)
	if _, err = repo.DB().Exec(`INSERT INTO generation_records
    (path,codec,encoder,encoder_version,bitrate_kbps,mode,size,mtime,content_sha256,created_at)
    VALUES(?,'mp3','encoder','1',320,'cbr',11,22,'hash','2026-01-01')`, file); err != nil {
		t.Fatal(err)
	}
	got, found, err := repo.EntryByPath(file)
	want := inventory.Entry{
		Path:        file,
		ParentPath:  work + "/disc",
		Name:        "track.mp3",
		Size:        11,
		Mtime:       22,
		ContentRev:  3,
		Format:      "mp3",
		Bitrate:     new(int32(320000)),
		Width:       new(50),
		Height:      new(60),
		DurationSec: new(7.5),
	}
	if err != nil || !found || !reflect.DeepEqual(got, want) {
		t.Fatalf("point=%+v found=%v err=%v", got, found, err)
	}
	for _, absent := range []string{work, file + ".missing", work + "/track.mp3"} {
		if _, found, err = repo.EntryByPath(absent); err != nil || found {
			t.Fatalf("whitelist %q found=%v err=%v", absent, found, err)
		}
	}
	entries, err := repo.ListEntriesUnderPath(file)
	if err != nil || len(entries) != 1 || !reflect.DeepEqual(entries[0], want) {
		t.Fatalf("file scope=%+v err=%v", entries, err)
	}
	entries, err = repo.ListEntriesUnderPath(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Path == root {
			t.Fatalf("library root leaked: %+v", entry)
		}
	}
	direct, found, err := repo.EntryByPath(prefix + "direct.jpg")
	if err != nil || !found || direct.ParentPath != root {
		t.Fatalf("root file=%+v found=%v err=%v", direct, found, err)
	}
	listed, err := repo.ListLibraryDirs(root)
	if err != nil || len(listed) != 1 || listed[0].FileCount != 1 || listed[0].AudioFileCount != 1 ||
		listed[0].Name != "RJ000001%_日" || listed[0].AddedAt.Unix() != 123 {
		t.Fatalf("overview=%+v err=%v", listed, err)
	}
	observed, err := repo.ObservedAudioEntries(work + "/")
	credential := &inventory.GeneratedCredential{Codec: "mp3", BitrateKbps: 320, Mode: "cbr", Size: 11, Mtime: 22}
	if err != nil || len(observed) != 1 || observed[0].Path != file ||
		!reflect.DeepEqual(observed[0].Generated, credential) {
		t.Fatalf("observed=%+v err=%v", observed, err)
	}
	pointObserved, err := repo.ObservedAudioEntries(file)
	if err != nil || !reflect.DeepEqual(pointObserved, observed) {
		t.Fatalf("file-root observed=%+v err=%v; want %+v", pointObserved, err, observed)
	}
}

func TestObservedAudioEntriesByRootMatchesPerRootReads(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	inventoryfixture.SeedInventory(t, repo,
		inventoryfixture.Row{Path: "/m/a", RootPath: "/m", IsDir: true},
		inventoryfixture.Row{Path: "/m/a/disc/1.mp3", RootPath: "/m", Size: 1, Mtime: 2, Format: "mp3"},
		inventoryfixture.Row{Path: "/m/a/0.mp3", RootPath: "/m", Size: 1, Mtime: 2, Format: "mp3"},
		inventoryfixture.Row{Path: "/m/a b/1.mp3", RootPath: "/m", Size: 1, Mtime: 2, Format: "mp3"},
		inventoryfixture.Row{Path: "/m/100%_x/1.mp3", RootPath: "/m", Size: 1, Mtime: 2, Format: "mp3"},
		inventoryfixture.Row{Path: "/m/single.mp3", RootPath: "/m", Size: 1, Mtime: 2, Format: "mp3"},
	)
	if _, err = repo.DB().Exec(`INSERT INTO generation_records
    (path,codec,encoder,encoder_version,bitrate_kbps,mode,size,mtime,content_sha256,created_at)
    VALUES('/m/a/0.mp3','mp3','e','1',320,'cbr',1,2,'h','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	roots := []string{"/m/a", "/m/a b", "/m/100%_x", "/m/single.mp3", "/m/gone", "/m/a/disc"}
	batched, err := repo.ObservedAudioEntriesByRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range roots {
		want, err := repo.ObservedAudioEntries(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(want) == 0 {
			want = nil
		}
		if !reflect.DeepEqual(batched[root], want) {
			t.Fatalf("%s: batched %+v, per-root %+v", root, batched[root], want)
		}
	}
}

func TestLocalCoverReadUsesCandidateFilesAndStableVersions(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	lib, err := repo.CreateLibrary("music", "/music")
	if err != nil {
		t.Fatal(err)
	}
	var staged []inventory.StagingEntry
	for _, name := range []string{"RJ000001 a", "RJ000001 b", "RJ000002 empty"} {
		staged = append(staged, inventory.StagingEntry{SessionID: "scan", RootPath: "/music", Path: "/music/" + name,
			ParentPath: "/music", Name: name, IsDir: true, IsWorkRoot: true})
	}
	if err = sqlite.NewScanStaging(repo).WriteStagingEntries("scan", staged); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.MergeStaging("scan", lib.ID, "/music"); err != nil {
		t.Fatal(err)
	}
	ids := []works.WorkCode{"RJ00000001", "RJ00000002"}
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{
			Path:       "/music/RJ000001 b/cover.png",
			RootPath:   "/music",
			Size:       11,
			Mtime:      7,
			ContentRev: 2,
		},
		inventoryfixture.Row{Path: "/music/RJ000001 a/not-cover.jpg", RootPath: "/music"},
	)
	covers, err := repo.LocalCovers(t.Context(), ids)
	if err != nil || len(covers) != 1 || covers[ids[0]].RelPath != "cover.png" || covers[ids[0]].Version != "2-7-11" ||
		covers[ids[0]].Size != 11 || covers[ids[0]].Mtime != 7 || covers[ids[0]].ContentRev != 2 {
		t.Fatalf("cover candidates=%+v err=%v", covers, err)
	}
	first := covers[ids[0]]
	inventoryfixture.SeedInventory(t, repo,
		inventoryfixture.Row{Path: "/music/RJ000001 a/cover.webp", RootPath: "/music"},
		inventoryfixture.Row{Path: "/music/RJ000001 a/cover.jpg", RootPath: "/music", Size: 4, Mtime: 8, ContentRev: 3})
	covers, err = repo.LocalCovers(t.Context(), ids)
	winner := covers[ids[0]]
	if err != nil || winner.CopyID == first.CopyID || winner.RelPath != "cover.jpg" || winner.Version != "3-8-4" {
		t.Fatalf("stable winner=%+v err=%v", covers, err)
	}
}
