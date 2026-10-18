package filesystem_test

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
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
	renameErr := os.Rename(source, renamed)
	switch runtime.GOOS {
	case "linux":
		if renameErr != nil {
			t.Fatal(renameErr)
		}
		if writeErr := os.WriteFile(source, []byte("replacement"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		data, readErr := io.ReadAll(file)
		if readErr != nil || string(data) != "original" {
			t.Fatalf("open identity lost: %q %v", data, readErr)
		}
	case "windows":
		if renameErr == nil {
			t.Fatal("open source did not prevent rename")
		}
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if removeErr := os.Remove(source); removeErr != nil {
			t.Fatal(removeErr)
		}
	default:
		t.Skip("platform-specific handle semantics")
	}
}
