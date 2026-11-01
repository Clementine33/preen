//go:build windows

package filesystem_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
)

// longDir returns a directory whose own path exceeds 260 characters, built
// from several legal components, and creates it.
func longDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for len(dir) <= 280 {
		dir = filepath.Join(dir, strings.Repeat("d", 60))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestWindowsLongPathRenameNoReplace(t *testing.T) {
	dir := longDir(t)
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.RenameNoReplace(src, dst); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := os.WriteFile(src, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.RenameNoReplace(src, dst); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("existing dst error = %v; want ErrExist", err)
	}
}

func TestWindowsLongPathConversionMove(t *testing.T) {
	long := filepath.Join(longDir(t), "long.txt")
	if len(filepath.Dir(long)) <= 260 {
		t.Fatalf("parent too short: %d", len(filepath.Dir(long)))
	}
	short := filepath.Join(t.TempDir(), "short.txt")
	if err := os.WriteFile(short, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, step := range [][2]string{{short, long}, {long, short}} {
		result, err := filesystem.ConversionMoveForTest(step[0], step[1])
		if err != nil || !result.TargetCreated || !result.SourceRemoved {
			t.Fatalf("move %d->%d chars = %+v, %v", len(step[0]), len(step[1]), result, err)
		}
	}
}
