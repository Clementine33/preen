package coverdelivery_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

func TestCoverInventoryRetryOnlyChangesPublishedAndRemovedCoverRows(t *testing.T) {
	_, repo, dl, req, folders := deliveryFixture(t)
	req.Refresh = true
	root := req.Targets[0].LibraryRoot
	old := filepath.ToSlash(filepath.Join(folders[0], "cover.jpg"))
	audio := filepath.ToSlash(filepath.Join(folders[0], "audio.mp3"))
	untouched := filepath.ToSlash(filepath.Join(folders[1], "other.png"))
	if err := os.WriteFile(old, dl.data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := repo.SyncObservedInventory(root, nil, []inventory.InventoryFile{
		{Path: old, Size: int64(len(dl.data)), Mtime: 1},
		{Path: audio, Size: 200, Mtime: 2},
		{Path: untouched, Size: 300, Mtime: 3},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().
		Exec(`UPDATE entries SET bitrate=128,duration_sec=25,width=123,content_rev=7 WHERE path=?`, audio); err != nil {
		t.Fatal(err)
	}
	var bindingBefore string
	if err := repo.DB().
		QueryRow(`SELECT rel_path FROM work_copies WHERE id=?`, req.Targets[0].CopyID).
		Scan(&bindingBefore); err != nil {
		t.Fatal(err)
	}
	fail := true
	svc, err := coverdelivery.New(coverdelivery.Dependencies{
		Store:      repo,
		Downloader: dl,
		Files:      filesystem.CoverFiles{},
		Acquire:    admission.NewGate(nil).BeginManual,
		SyncInventory: func(_ context.Context, target coverdelivery.Target, result coverdelivery.Publication) error {
			if fail {
				return fmt.Errorf("inventory unavailable")
			}
			return repo.SyncObservedInventory(
				target.LibraryRoot,
				result.Removed,
				[]inventory.InventoryFile{*result.File},
				nil,
			)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(t.Context(), "exec")
	if err != nil || report.Counts["failed"] != 2 || report.Items[0].Published != "cover.png" {
		t.Fatalf("failure: %+v %v", report, err)
	}
	if _, err = os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old cover not removed: %v", err)
	}
	if _, err = os.Stat(filepath.Join(folders[0], ".onsei-cover-publish.json")); err != nil {
		t.Fatal("retry journal lost", err)
	}
	fail = false
	if err = svc.Retry(t.Context(), "exec"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err = svc.Report(t.Context(), "exec")
	if err != nil || report.Counts["succeeded"] != 2 {
		t.Fatalf("retry: %+v %v", report, err)
	}
	var count int
	if err = repo.DB().
		QueryRow(`SELECT COUNT(*) FROM entries WHERE path=?`, old).
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatalf("stale old row: %d %v", count, err)
	}
	for _, folder := range folders {
		if err = repo.DB().
			QueryRow(`SELECT COUNT(*) FROM entries WHERE path=?`, filepath.ToSlash(filepath.Join(folder, "cover.png"))).
			Scan(&count); err != nil ||
			count != 1 {
			t.Fatalf("published row: %d %v", count, err)
		}
		if _, err = os.Stat(filepath.Join(folder, ".onsei-cover-publish.json")); !os.IsNotExist(err) {
			t.Fatalf("acknowledged journal: %v", err)
		}
	}
	assertUnchangedCoverNeighbours(t, repo, audio, untouched, req.Targets[0].CopyID, bindingBefore)
}

func assertUnchangedCoverNeighbours(
	t *testing.T,
	repo *sqlite.Repository,
	audio, untouched, copyID, bindingBefore string,
) {
	t.Helper()
	var size, mtime, bitrate, width, rev int
	var duration float64
	if err := repo.DB().
		QueryRow(`SELECT size,mtime,bitrate,width,duration_sec,content_rev FROM entries WHERE path=?`, audio).
		Scan(&size, &mtime, &bitrate, &width, &duration, &rev); err != nil {
		t.Fatal(err)
	}
	if size != 200 || mtime != 2 || bitrate != 128 || width != 123 || duration != 25 || rev != 7 {
		t.Fatal("audio facts changed")
	}
	if err := repo.DB().
		QueryRow(`SELECT size FROM entries WHERE path=?`, untouched).
		Scan(&size); err != nil ||
		size != 300 {
		t.Fatalf("unrelated image: %d %v", size, err)
	}
	var bindingAfter string
	if err := repo.DB().
		QueryRow(`SELECT rel_path FROM work_copies WHERE id=?`, copyID).
		Scan(&bindingAfter); err != nil ||
		bindingAfter != bindingBefore {
		t.Fatalf("binding changed: %s %v", bindingAfter, err)
	}
}
