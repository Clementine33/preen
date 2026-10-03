package filesystem_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/works/coverdelivery"
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
	id, err := filesystem.CoverDirectoryIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	target := coverdelivery.Target{
		CopyID:      "copy",
		WorkCode:    "RJ00000001",
		LibraryRoot: root,
		RelPath:     "RJ00000001",
		Identity:    id,
	}
	files := filesystem.CoverFiles{}
	result, err := files.Publish(t.Context(), target, b.Bytes(), false)
	if err != nil || !result.Skipped {
		t.Fatalf("fill overwrote existing cover: %+v %v", result, err)
	}
	result, err = files.Publish(t.Context(), target, b.Bytes(), true)
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
	id, err := filesystem.CoverDirectoryIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
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
	result, err := files.Publish(t.Context(), target, data, false)
	if err != nil || !result.Skipped {
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
	result, err := (filesystem.CoverFiles{}).Publish(t.Context(), target, data, false)
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
	if _, err := files.Publish(t.Context(), target, data, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cover.jpg"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Publish(t.Context(), target, data, false); coverdelivery.Code(err) != "COVER_CONFLICT" {
		t.Fatalf("multiple covers accepted: %v", err)
	}
	if _, err := files.Publish(t.Context(), target, data, true); err != nil {
		t.Fatal(err)
	}
}
func TestCoverRefusesReplacedDirectoryAndSymlinks(t *testing.T) {
	target, data := coverFixture(t)
	dir := filepath.Join(target.LibraryRoot, target.RelPath)
	if err := os.Rename(dir, dir+"-moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := (filesystem.CoverFiles{}).Publish(
		t.Context(),
		target,
		data,
		true,
	); coverdelivery.Code(
		err,
	) != "INPUT_CHANGED" {
		t.Fatalf("replaced directory accepted: %v", err)
	}
	target.Identity = ""
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "cover.png")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := (filesystem.CoverFiles{}).Publish(
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
	if _, err := (filesystem.CoverFiles{}).Publish(t.Context(), target, data, true); err == nil {
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
	if _, err = (filesystem.CoverFiles{}).Publish(
		t.Context(),
		target,
		data,
		true,
	); coverdelivery.Code(
		err,
	) != "COVER_CONFLICT" {
		t.Fatalf("changed old path accepted: %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir, "cover.jpg")); err != nil || !info.IsDir() {
		t.Fatal("old name directory removed")
	}
}
