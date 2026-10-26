package coverdelivery_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

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
	inventoryfixture.SeedInventory(t, repo, inventoryfixture.Row{
		Path:        audio,
		RootPath:    root,
		Size:        200,
		Mtime:       2,
		Bitrate:     new(int32(128)),
		DurationSec: new(float64(25)),
		Width:       new(int(123)),
		Height:      new(int(456)),
		ContentRev:  7,
	})
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
	if count, readErr := repo.RootExistsInInventory(old); readErr != nil ||
		count {
		t.Fatalf("stale old row: %v %v", count, readErr)
	}
	for _, folder := range folders {
		assertCoverMediaCandidate(t, repo, filepath.ToSlash(filepath.Join(folder, "cover.png")))
		if count, readErr := repo.RootExistsInInventory(
			filepath.ToSlash(filepath.Join(folder, "cover.png")),
		); readErr != nil ||
			!count {
			t.Fatalf("published row: %v %v", count, readErr)
		}
		if _, err = os.Stat(filepath.Join(folder, ".onsei-cover-publish.json")); !os.IsNotExist(err) {
			t.Fatalf("acknowledged journal: %v", err)
		}
	}
	assertUnchangedCoverNeighbours(t, repo, audio, untouched, req.Targets[0].CopyID, bindingBefore)
}

func assertCoverMediaCandidate(t *testing.T, repo *sqlite.Repository, cover string) {
	t.Helper()
	entry, found, err := repo.EntryByPath(cover)
	if err != nil || !found || entry.Format != "image/png" {
		t.Fatalf("published cover facts: %+v found=%v err=%v", entry, found, err)
	}
	candidates, err := sqlite.NewScanStaging(repo).EntriesMissingMediaFacts("", 20)
	if err != nil {
		t.Fatal(err)
	}
	candidate := false
	for _, c := range candidates {
		candidate = candidate || c.Path == cover
	}
	if !candidate {
		t.Fatalf("published cover missing from media candidates: %+v", candidates)
	}
}

func assertUnchangedCoverNeighbours(
	t *testing.T,
	repo *sqlite.Repository,
	audio, untouched, copyID, bindingBefore string,
) {
	t.Helper()
	entry, found, err := repo.EntryByPath(audio)
	if err != nil || !found {
		t.Fatalf("read audio: found=%v err=%v", found, err)
	}
	if entry.Size != 200 || entry.Mtime != 2 || entry.Bitrate == nil || *entry.Bitrate != 128 || entry.Width == nil ||
		*entry.Width != 123 ||
		entry.DurationSec == nil ||
		*entry.DurationSec != 25 ||
		entry.ContentRev != 7 {
		t.Fatalf("audio facts changed: %+v", entry)
	}
	other, found, err := repo.EntryByPath(untouched)
	if err != nil || !found || other.Size != 300 {
		t.Fatalf("unrelated image: %+v found=%v err=%v", other, found, err)
	}
	var bindingAfter string
	if err := repo.DB().
		QueryRow(`SELECT rel_path FROM work_copies WHERE id=?`, copyID).
		Scan(&bindingAfter); err != nil ||
		bindingAfter != bindingBefore {
		t.Fatalf("binding changed: %s %v", bindingAfter, err)
	}
}
