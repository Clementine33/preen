package dirrename_test

import (
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
)

func TestRelocateDirectoryPreservesFactsCredentialsCopiesAndSharedMembers(t *testing.T) {
	f := newRenameRun(t)
	root := detailRoot(f.member)
	source := relocateUnicodeSource(t, f)
	target := filepath.ToSlash(filepath.Join(root, "[RJ00000001] 声_%"))
	child := source + "/音_%/track.opus"
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.repo.DB().Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	inventoryfixture.SeedInventory(t, f.repo, inventoryfixture.Row{
		Path:        child,
		RootPath:    source,
		Size:        123,
		Mtime:       42,
		ContentRev:  7,
		Bitrate:     new(int32(128000)),
		DurationSec: new(8.5),
	})
	inventoryfixture.SeedInventory(t, f.repo, inventoryfixture.Row{Path: source + "suffix/keep"})
	exec(`INSERT INTO generation_records VALUES(?,'opus','ffmpeg','9.0.1',128,'vbr',123,42,'sha','date')`, child)
	var originalMemberID string
	if err := f.repo.DB().
		QueryRow(`SELECT member_id FROM workset_members WHERE workset_id=?`, f.worksetID).
		Scan(&originalMemberID); err != nil {
		t.Fatal(err)
	}
	var copyID string
	if err := f.repo.DB().
		QueryRow(`SELECT id FROM work_copies WHERE library_id=? AND rel_path=?`, f.libraryID, f.member.RelPath).
		Scan(&copyID); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.RelocateDirectory(t.Context(), root, source, target); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.RelocateDirectory(t.Context(), root, source, target); err != nil {
		t.Fatal("not idempotent", err)
	}
	entry, found, err := f.repo.EntryByPath(target + "/音_%/track.opus")
	if err != nil || !found {
		t.Fatalf("relocated inventory: found=%v err=%v", found, err)
	}
	if entry.ParentPath != target+"/音_%" || entry.Name != "track.opus" || entry.Size != 123 || entry.Mtime != 42 ||
		entry.ContentRev != 7 ||
		entry.Bitrate == nil ||
		*entry.Bitrate != 128000 ||
		entry.DurationSec == nil ||
		*entry.DurationSec != 8.5 {
		t.Fatal("changed unrelated inventory facts")
	}
	var rootName, credential, rel, memberPath, memberName, memberID string
	roots, err := f.repo.ListEntriesUnderPath(target)
	if err != nil || len(roots) == 0 {
		t.Fatalf("relocated directory: entries=%+v err=%v", roots, err)
	}
	rootName = roots[0].Name
	if err := f.repo.DB().
		QueryRow(`SELECT content_sha256 FROM generation_records WHERE path=?`, target+"/音_%/track.opus").
		Scan(&credential); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.DB().QueryRow(`SELECT rel_path FROM work_copies WHERE id=?`, copyID).Scan(&rel); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.DB().
		QueryRow(`SELECT folder_path,folder_name,member_id FROM workset_members WHERE workset_id=?`, f.worksetID).
		Scan(&memberPath, &memberName, &memberID); err != nil {
		t.Fatal(err)
	}
	if rootName != filepath.Base(target) || credential != "sha" || rel != filepath.Base(target) ||
		memberPath != target ||
		memberName != filepath.Base(target) ||
		memberID != originalMemberID {
		t.Fatal("path identity was not preserved")
	}
	if found, err := f.repo.RootExistsInInventory(source + "suffix/keep"); err != nil || !found {
		t.Fatalf("prefix boundary: %v %v", found, err)
	}
}
func TestRelocateConflictRollsBackAllPathChanges(t *testing.T) {
	f := newRenameRun(t)
	source := f.member.FolderPath
	target := filepath.ToSlash(filepath.Join(detailRoot(f.member), "[RJ00000001] title"))
	inventoryfixture.SeedInventory(t, f.repo, inventoryfixture.Row{Path: target})
	if err := f.repo.RelocateDirectory(t.Context(), detailRoot(f.member), source, target); err == nil {
		t.Fatal("overwrote inventory target")
	}
	old, err := f.repo.RootExistsInInventory(source)
	if err != nil {
		t.Fatal(err)
	}
	occupiedEntry, occupied, err := f.repo.EntryByPath(target)
	if err != nil {
		t.Fatal(err)
	}
	var members int
	if err := f.repo.DB().
		QueryRow(`SELECT count(*) FROM workset_members WHERE folder_path=?`, source).
		Scan(&members); err != nil {
		t.Fatal(err)
	}
	if !old || !occupied || occupiedEntry.Name != filepath.Base(target) || members != 1 {
		t.Fatalf("partial migration: %v %v %d", old, occupied, members)
	}
}

func relocateUnicodeSource(t *testing.T, f renameRun) string {
	t.Helper()
	root := detailRoot(f.member)
	original := f.member.FolderPath
	source := filepath.ToSlash(filepath.Join(root, "RJ000001 原_%😀"))
	if err := filesystem.RenameNoReplace(original, source); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.RelocateDirectory(t.Context(), root, original, source); err != nil {
		t.Fatal(err)
	}
	f.member.FolderPath = source
	f.member.RelPath = filepath.Base(source)
	f.member.FolderName = filepath.Base(source)
	return source
}
