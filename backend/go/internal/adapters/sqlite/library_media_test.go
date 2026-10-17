package sqlite_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/library"
)

func TestMediaCandidateBatchCapturesOwningLibraryLimit(t *testing.T) {
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	lib, err := repo.CreateLibrary("SSD", "/ssd")
	if err != nil {
		t.Fatal(err)
	}
	libs := library.NewService(repo, nil, repo, nil)
	class := library.StorageSSD
	if _, err = libs.Patch(lib.ID, nil, nil, &class); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.CreateLibrary("HDD", "/hdd"); err != nil {
		t.Fatal(err)
	}
	win, err := repo.CreateLibrary("Windows SSD", "C:/Music")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = libs.Patch(win.ID, nil, nil, &class); err != nil {
		t.Fatal(err)
	}
	// Inventory keeps the old spelling when a root identity is unchanged.
	renamedRoot := "c:/music"
	if _, err = libs.Patch(win.ID, nil, &renamedRoot, nil); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{"/ssd", "/hdd", "/unowned", "C:/Music"} {
		if _, err = repo.DB().
			Exec(`INSERT INTO entries(path,root_path,parent_path,name,is_dir,size,mtime,format) VALUES(?,?,?,'a.mp3',0,1,1,'audio/mpeg')`, root+"/a.mp3", root, root); err != nil {
			t.Fatal(err)
		}
	}
	store := sqlite.NewScanStaging(repo)
	batch, err := store.EntriesMissingMediaFacts("", 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 4 {
		t.Fatalf("batch=%v", batch)
	}
	class = library.StorageHDD
	if _, err = libs.Patch(lib.ID, nil, nil, &class); err != nil {
		t.Fatal(err)
	}
	for _, c := range batch {
		want := library.StorageHDD.MediaWorkers(runtime.NumCPU())
		if c.Path == "/ssd/a.mp3" || c.Path == "C:/Music/a.mp3" {
			want = library.StorageSSD.MediaWorkers(runtime.NumCPU())
		}
		if c.ProbeWorkers != want {
			t.Fatalf("candidate %+v: want captured width %d", c, want)
		}
	}
	next, err := store.EntriesMissingMediaFacts("", 64)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range next {
		want := library.StorageHDD.MediaWorkers(runtime.NumCPU())
		if c.Path == "C:/Music/a.mp3" {
			want = library.StorageSSD.MediaWorkers(runtime.NumCPU())
		}
		if c.ProbeWorkers != want {
			t.Fatalf("new batch ignored library identity or storage change: %+v", c)
		}
	}
}
