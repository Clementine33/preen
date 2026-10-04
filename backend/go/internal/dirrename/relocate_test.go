package dirrename_test

import (
	"path/filepath"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/librarypath"
)

func TestRelocateDirectoryPreservesFactsCredentialsCopiesAndOtherWorksets(t *testing.T) {
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
	exec(
		`INSERT INTO entries(path,root_path,parent_path,name,path_posix,size,mtime,content_rev,bitrate,duration_sec) VALUES(?,?,?,?,?,123,42,7,128000,8.5)`,
		child,
		source,
		source+"/音_%",
		"track.opus",
		child,
	)
	exec(`INSERT INTO entries(path,name) VALUES(?,?)`, source+"suffix/keep", "keep")
	exec(`INSERT INTO generation_records VALUES(?,'opus','ffmpeg','9.0.1',128,'vbr',123,42,'sha','date')`, child)
	exec(
		`INSERT INTO worksets(id,library_id,operation_type,root_path,root_path_key) VALUES('other',?,'conversion',?,?)`,
		f.libraryID,
		root,
		librarypath.RootPathKey(root),
	)
	exec(
		`INSERT INTO workset_members(workset_id,member_id,member_index,rel_path,folder_path,folder_name) VALUES('other','stable',0,?,?,?)`,
		f.member.RelPath,
		source,
		f.member.FolderName,
	)
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
	var storedRoot, parent, name, posix string
	var size, mtime, rev, bitrate int
	var duration float64
	if err := f.repo.DB().
		QueryRow(`SELECT root_path,parent_path,name,path_posix,size,mtime,content_rev,bitrate,duration_sec FROM entries WHERE path=?`, target+"/音_%/track.opus").
		Scan(&storedRoot, &parent, &name, &posix, &size, &mtime, &rev, &bitrate, &duration); err != nil {
		t.Fatal(err)
	}
	if storedRoot != target || parent != target+"/音_%" || name != "track.opus" || posix != target+"/音_%/track.opus" ||
		size != 123 ||
		mtime != 42 ||
		rev != 7 ||
		bitrate != 128000 ||
		duration != 8.5 {
		t.Fatal("changed unrelated inventory facts")
	}
	var rootName, credential, rel, memberPath, memberName, memberID string
	if err := f.repo.DB().QueryRow(`SELECT name FROM entries WHERE path=?`, target).Scan(&rootName); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.DB().
		QueryRow(`SELECT content_sha256 FROM generation_records WHERE path=?`, target+"/音_%/track.opus").
		Scan(&credential); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.DB().QueryRow(`SELECT rel_path FROM work_copies WHERE id=?`, copyID).Scan(&rel); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.DB().
		QueryRow(`SELECT folder_path,folder_name,member_id FROM workset_members WHERE workset_id='other'`).
		Scan(&memberPath, &memberName, &memberID); err != nil {
		t.Fatal(err)
	}
	if rootName != filepath.Base(target) || credential != "sha" || rel != filepath.Base(target) ||
		memberPath != target ||
		memberName != filepath.Base(target) ||
		memberID != "stable" {
		t.Fatal("path identity was not preserved")
	}
	var unrelated int
	if err := f.repo.DB().
		QueryRow(`SELECT count(*) FROM entries WHERE path=?`, source+"suffix/keep").
		Scan(&unrelated); err != nil ||
		unrelated != 1 {
		t.Fatalf("prefix boundary: %d %v", unrelated, err)
	}
}
func TestRelocateConflictRollsBackAllPathChanges(t *testing.T) {
	f := newRenameRun(t)
	source := f.member.FolderPath
	target := filepath.ToSlash(filepath.Join(detailRoot(f.member), "[RJ00000001] title"))
	if _, err := f.repo.DB().Exec(`INSERT INTO entries(path,name) VALUES(?,'occupied')`, target); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.RelocateDirectory(t.Context(), detailRoot(f.member), source, target); err == nil {
		t.Fatal("overwrote inventory target")
	}
	var old, occupied, members int
	if err := f.repo.DB().QueryRow(`SELECT count(*) FROM entries WHERE path=?`, source).Scan(&old); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.DB().
		QueryRow(`SELECT count(*) FROM entries WHERE path=? AND name='occupied'`, target).
		Scan(&occupied); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.DB().
		QueryRow(`SELECT count(*) FROM workset_members WHERE folder_path=?`, source).
		Scan(&members); err != nil {
		t.Fatal(err)
	}
	if old != 1 || occupied != 1 || members != 1 {
		t.Fatalf("partial migration: %d %d %d", old, occupied, members)
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
