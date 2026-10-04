package filesystem_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
)

func TestIsResolvedWithinRoot(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "library")
	insideDir := filepath.Join(root, "album")
	outsideDir := filepath.Join(workspace, "outside")
	if err := os.MkdirAll(insideDir, 0o755); err != nil {
		t.Fatalf("create inside directory: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("create outside directory: %v", err)
	}
	insideFile := filepath.Join(insideDir, "inside.flac")
	outsideFile := filepath.Join(outsideDir, "outside.flac")
	if err := os.WriteFile(insideFile, []byte("audio"), 0o644); err != nil {
		t.Fatalf("create inside file: %v", err)
	}
	if err := os.WriteFile(outsideFile, []byte("audio"), 0o644); err != nil {
		t.Fatalf("create outside file: %v", err)
	}

	within, err := filesystem.IsResolvedWithinRoot(root, insideFile)
	if err != nil {
		t.Fatalf("IsResolvedWithinRoot inside file: %v", err)
	}
	if !within {
		t.Fatal("inside file should resolve within root")
	}

	// On Windows the filesystem is case-insensitive but filepath.EvalSymlinks
	// does not canonicalize letter case, so a case-swapped inside path must
	// still resolve within the root (mirrors IsWithinRoot's case folding).
	if runtime.GOOS == "windows" {
		caseSwapped := filepath.Join(
			filepath.Dir(insideDir),
			strings.ToUpper(filepath.Base(insideDir)),
			filepath.Base(insideFile),
		)
		within, err = filesystem.IsResolvedWithinRoot(root, caseSwapped)
		if err != nil {
			t.Fatalf("IsResolvedWithinRoot case-swapped inside file: %v", err)
		}
		if !within {
			t.Fatal("case-swapped inside file should resolve within root on Windows")
		}
	}

	linkPath := filepath.Join(root, "linked")
	if symlinkErr := os.Symlink(outsideDir, linkPath); symlinkErr != nil {
		t.Skipf("symlinks unavailable: %v", symlinkErr)
	}
	within, err = filesystem.IsResolvedWithinRoot(root, filepath.Join(linkPath, filepath.Base(outsideFile)))
	if err != nil {
		t.Fatalf("IsResolvedWithinRoot symlink escape: %v", err)
	}
	if within {
		t.Fatal("file reached through an escaping symlink should not resolve within root")
	}
}
