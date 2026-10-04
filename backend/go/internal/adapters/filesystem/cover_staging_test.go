package filesystem_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/works/coverdelivery"
)

func TestCoverStagingConfinesSavedPathsAndBoundsReads(t *testing.T) {
	dir := t.TempDir()
	stage, err := filesystem.NewCoverStaging(dir)
	if err != nil {
		t.Fatal(err)
	}
	const job = "00000000-0000-4000-8000-000000000001"
	path, err := stage.Write(job, []byte("download"))
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, job+".image") {
		t.Fatalf("stored staging path changed: %s", path)
	}
	if _, err = stage.Read("other", path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("another job read the saved file: %v", err)
	}
	if err = stage.Remove("other", path); err != nil {
		t.Fatal(err)
	}
	if data, readErr := os.ReadFile(path); readErr != nil || string(data) != "download" {
		t.Fatalf("another job removed the saved file: %q, %v", data, readErr)
	}
	if err = os.Truncate(path, coverdelivery.MaxBytes+100); err != nil {
		t.Fatal(err)
	}
	data, err := stage.Read(job, path)
	if err != nil || len(data) != coverdelivery.MaxBytes+1 {
		t.Fatalf("staged image read exceeded the validation budget: %d, %v", len(data), err)
	}
	if err = stage.Remove(job, path); err != nil {
		t.Fatal(err)
	}
	if err = stage.Remove(job, path); err != nil {
		t.Fatalf("removing an absent stage: %v", err)
	}
}

func TestCoverStagingCleansOnlyInactiveJobFiles(t *testing.T) {
	dir := t.TempDir()
	stage, err := filesystem.NewCoverStaging(dir)
	if err != nil {
		t.Fatal(err)
	}
	const active = "00000000-0000-4000-8000-000000000001"
	const orphan = "00000000-0000-4000-8000-000000000002"
	const directory = "00000000-0000-4000-8000-000000000003.image"
	keep := []string{active + ".image", active + ".image.tmp", "notes", "invalid.image", orphan + ".image.extra"}
	remove := []string{orphan + ".image", orphan + ".image.tmp"}
	for _, names := range [][]string{keep, remove} {
		for _, name := range names {
			if err = os.WriteFile(filepath.Join(dir, name), []byte("bytes"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = os.Mkdir(filepath.Join(dir, directory), 0700); err != nil {
		t.Fatal(err)
	}
	if err = stage.Cleanup(map[string]bool{active: true}); err != nil {
		t.Fatal(err)
	}
	for _, name := range append(keep, directory) {
		if _, err = os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("cleanup lost %s: %v", name, err)
		}
	}
	for _, name := range remove {
		if _, err = os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("orphan retained: %s, %v", name, err)
		}
	}
}

func TestCoverStagingFailedReplacementCleansTemporaryBytes(t *testing.T) {
	dir := t.TempDir()
	stage, err := filesystem.NewCoverStaging(dir)
	if err != nil {
		t.Fatal(err)
	}
	const job = "00000000-0000-4000-8000-000000000001"
	path := filepath.Join(dir, job+".image")
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = stage.Write(job, []byte("download")); err == nil {
		t.Fatal("staging overwrote a directory")
	}
	if _, err = os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("failed replacement leaked temporary bytes: %v", err)
	}
	if info, statErr := os.Stat(path); statErr != nil || !info.IsDir() {
		t.Fatalf("failed replacement lost the existing directory: %v", statErr)
	}
}
