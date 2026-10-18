package filesystem_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

func TestCoverPublicationPreservesFillAndReplacesOnlyCoverFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "RJ00000001")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cover.jpg"), b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.png"), b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	id := "legacy-identity"
	target := coverdelivery.Target{
		CopyID:      "copy",
		WorkCode:    "RJ00000001",
		LibraryRoot: root,
		RelPath:     "RJ00000001",
		Identity:    id,
	}
	files := filesystem.CoverFiles{}
	result, err := publishCoverForTest(files, t.Context(), target, b.Bytes(), false)
	if err != nil || !result.Skipped {
		t.Fatalf("fill overwrote existing cover: %+v %v", result, err)
	}
	result, err = publishCoverForTest(files, t.Context(), target, b.Bytes(), true)
	if err != nil || result.Name != "cover.png" {
		t.Fatalf("refresh: %+v %v", result, err)
	}
	if _, err = os.Stat(filepath.Join(dir, "cover.jpg")); !os.IsNotExist(err) {
		t.Fatalf("old extension survived: %v", err)
	}
	if _, err = os.Stat(filepath.Join(dir, "other.png")); err != nil {
		t.Fatal("unrelated image removed")
	}
}

func coverFixture(t *testing.T) (coverdelivery.Target, []byte) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "RJ00000001")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	id := "legacy-identity"
	return coverdelivery.Target{LibraryRoot: root, RelPath: "RJ00000001", Identity: id}, b.Bytes()
}
func TestPublishedCoverRecoveryIsVisibleBeforeCleanup(t *testing.T) {
	target, data := coverFixture(t)
	dir := filepath.Join(target.LibraryRoot, target.RelPath)
	for _, name := range []string{"cover.png", "cover.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	sum := sha256.Sum256(data)
	raw, err := json.Marshal(
		map[string]any{
			"name":   "cover.png",
			"temp":   ".onsei-cover-test.tmp",
			"digest": hex.EncodeToString(sum[:]),
			"old":    []string{"cover.jpg"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, ".onsei-cover-publish.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	files := filesystem.CoverFiles{}
	result, err := publishCoverForTest(files, t.Context(), target, data, false)
	if err != nil || result.Name != "cover.png" {
		t.Fatalf("recovery: %+v %v", result, err)
	}
	if _, err = os.Stat(filepath.Join(dir, "cover.jpg")); !os.IsNotExist(err) {
		t.Fatal("recovery left old extension")
	}
	if _, err = os.Stat(filepath.Join(dir, ".onsei-cover-publish.json")); !os.IsNotExist(err) {
		t.Fatal("journal survived recovery")
	}
}
func TestIncompletePublicationPreservesOldCover(t *testing.T) {
	target, data := coverFixture(t)
	dir := filepath.Join(target.LibraryRoot, target.RelPath)
	if err := os.WriteFile(filepath.Join(dir, "cover.jpg"), data, 0600); err != nil {
		t.Fatal(err)
	}
	journal := []byte(
		`{"name":"cover.png","temp":".onsei-cover-test.tmp","digest":"not-published","old":["cover.jpg"]}`,
	)
	if err := os.WriteFile(filepath.Join(dir, ".onsei-cover-publish.json"), journal, 0600); err != nil {
		t.Fatal(err)
	}
	existing, err := (filesystem.CoverFiles{}).Existing(target)
	if err != nil || existing == nil || !existing.NeedsRecovery {
		t.Fatalf("pending publication invisible: %+v %v", existing, err)
	}
	result, err := publishCoverForTest(filesystem.CoverFiles{}, t.Context(), target, data, false)
	if err != nil || !result.Skipped {
		t.Fatalf("old cover lost: %+v %v", result, err)
	}
}
func TestCoverConflictAndDamagedImage(t *testing.T) {
	target, data := coverFixture(t)
	dir := filepath.Join(target.LibraryRoot, target.RelPath)
	files := filesystem.CoverFiles{}
	if err := os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := publishCoverForTest(files, t.Context(), target, data, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cover.jpg"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := publishCoverForTest(
		files,
		t.Context(),
		target,
		data,
		false,
	); coverdelivery.Code(
		err,
	) != "COVER_CONFLICT" {
		t.Fatalf("multiple covers accepted: %v", err)
	}
	if _, err := publishCoverForTest(files, t.Context(), target, data, true); err != nil {
		t.Fatal(err)
	}
}
func TestCoverAcceptsReplacedDirectoryAndRefusesSymlinks(t *testing.T) {
	target, data := coverFixture(t)
	dir := filepath.Join(target.LibraryRoot, target.RelPath)
	if err := os.Rename(dir, dir+"-moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := publishCoverForTest(filesystem.CoverFiles{}, t.Context(), target, data, true); err != nil {
		t.Fatalf("same-name replacement rejected: %v", err)
	}
	target.Identity = ""
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "cover.png")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "cover.png")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := publishCoverForTest(filesystem.CoverFiles{},
		t.Context(),
		target,
		data,
		true,
	); coverdelivery.Code(
		err,
	) != "COVER_CONFLICT" {
		t.Fatalf("symlink accepted: %v", err)
	}
}

func TestReadOnlyDirectoryKeepsOldCover(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires unprivileged POSIX permissions")
	}
	target, data := coverFixture(t)
	dir := filepath.Join(target.LibraryRoot, target.RelPath)
	if err := os.WriteFile(filepath.Join(dir, "cover.jpg"), data, 0600); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // A real read-only directory is the failure condition under test.
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // Restore traversal so TempDir cleanup can remove this directory.
	defer func() { _ = os.Chmod(dir, 0755) }()
	if _, err := publishCoverForTest(filesystem.CoverFiles{}, t.Context(), target, data, true); err == nil {
		t.Fatal("read-only publication succeeded")
	}
	existing, err := (filesystem.CoverFiles{}).Existing(target)
	if err != nil || existing == nil || existing.Name != "cover.jpg" {
		t.Fatalf("old file lost: %+v %v", existing, err)
	}
}

func TestRecoveryNeverRemovesDirectoryAtAnOldCoverName(t *testing.T) {
	target, data := coverFixture(t)
	dir := filepath.Join(target.LibraryRoot, target.RelPath)
	if err := os.WriteFile(filepath.Join(dir, "cover.png"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "cover.jpg"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cover.jpeg"), data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	raw, err := json.Marshal(
		map[string]any{
			"name":   "cover.png",
			"temp":   ".onsei-cover-test.tmp",
			"digest": hex.EncodeToString(sum[:]),
			"old":    []string{"cover.jpeg", "cover.jpg"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, ".onsei-cover-publish.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := publishCoverForTest(filesystem.CoverFiles{},
		t.Context(),
		target,
		data,
		true,
	)
	if coverdelivery.Code(err) != "COVER_CONFLICT" {
		t.Fatalf("changed old path accepted: %v", err)
	}
	if result.File == nil || result.Name != "cover.png" || len(result.Removed) != 1 ||
		result.Removed[0] != filepath.ToSlash(filepath.Join(dir, "cover.jpeg")) {
		t.Fatalf("partial facts lost: %+v", result)
	}
	if info, err := os.Stat(filepath.Join(dir, "cover.jpg")); err != nil || !info.IsDir() {
		t.Fatal("old name directory removed")
	}
}

func TestRecoveryDigestMismatchPreservesOldExtension(t *testing.T) {
	target, data := coverFixture(t)
	dir := filepath.Join(target.LibraryRoot, target.RelPath)
	for _, name := range []string{"cover.png", "cover.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	journal := []byte(`{"name":"cover.png","temp":".onsei-cover-test.tmp","digest":"wrong","old":["cover.jpg"]}`)
	if err := os.WriteFile(filepath.Join(dir, ".onsei-cover-publish.json"), journal, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := publishCoverForTest(
		filesystem.CoverFiles{},
		t.Context(),
		target,
		data,
		false,
	); coverdelivery.Code(
		err,
	) != "COVER_CONFLICT" {
		t.Fatalf("unconfirmed recovery: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cover.jpg")); err != nil {
		t.Fatal("old cover removed without digest confirmation", err)
	}
}

func TestNewCandidateKeepsUnacknowledgedOldInventoryDeletions(t *testing.T) {
	target, data := coverFixture(t)
	dir := filepath.Join(target.LibraryRoot, target.RelPath)
	if err := os.WriteFile(filepath.Join(dir, "cover.png"), data, 0600); err != nil {
		t.Fatal(err)
	}
	// The old extension was deleted before inventory sync failed, and a newer
	// candidate's journal was persisted but its image did not land.
	journal := []byte(
		`{"name":"cover.png","temp":".onsei-cover-test.tmp","digest":"new-candidate-not-landed","old":["cover.jpg"]}`,
	)
	if err := os.WriteFile(filepath.Join(dir, ".onsei-cover-publish.json"), journal, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := publishCoverForTest(filesystem.CoverFiles{}, t.Context(), target, data, true)
	if err != nil || result.File == nil || len(result.Removed) != 1 ||
		result.Removed[0] != filepath.ToSlash(filepath.Join(dir, "cover.jpg")) {
		t.Fatalf("lost retry delta: %+v %v", result, err)
	}
}

func publishCoverForTest(
	files filesystem.CoverFiles,
	ctx context.Context,
	target coverdelivery.Target,
	data []byte,
	refresh bool,
) (coverdelivery.Publication, error) {
	sum := sha256.Sum256(data)
	result, err := files.Publish(ctx, target, data, hex.EncodeToString(sum[:]), refresh)
	if err == nil {
		err = files.CompletePublication(target)
	}
	return result, err
}
