package filesystem_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
)

func TestConversionSourceHandleKeepsPlatformIdentity(t *testing.T) {
	root := t.TempDir()
	source, renamed := filepath.Join(root, "source.wav"), filepath.Join(root, "renamed.wav")
	if err := os.WriteFile(source, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := (filesystem.ConversionHandles{}).OpenSource(root, source)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	original, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	// Root.OpenFile permits rename on Windows through FILE_SHARE_DELETE.
	// The handle must keep the original object when its old path is replaced.
	if renameErr := os.Rename(source, renamed); renameErr != nil {
		t.Fatal(renameErr)
	}
	if writeErr := os.WriteFile(source, []byte("replacement"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "original" {
		t.Fatalf("open identity lost: %q %v", data, err)
	}
	current, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(original, current) || os.SameFile(current, replacement) {
		t.Fatal("open handle no longer identifies the original file")
	}
}
