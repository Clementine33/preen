package filesystem_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"testing/synctest"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
)

func recoveryFixture(t *testing.T) (string, string, string) {
	t.Helper()
	parent := filepath.Join(t.TempDir(), "category")
	root := filepath.Join(parent, "RJ000001")
	source := filepath.Join(root, "disc", "track.mp3")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	recoveryWrite(t, source, "original")
	return root, source, filepath.Join(parent, "Delete", "RJ000001", "disc", "track.mp3")
}

func recoveryWrite(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func recoveryContent(t *testing.T, name, want string) {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil || string(content) != want {
		t.Fatalf("%s: content = %q, err = %v; want %q", name, content, err, want)
	}
}

func TestRecoverPreservesWorkLayoutAndDirectory(t *testing.T) {
	root, source, want := recoveryFixture(t)
	got, err := filesystem.Recover(t.Context(), root, source)
	if err != nil || got != want {
		t.Fatalf("Recover = %q, %v; want %q", got, err, want)
	}
	recoveryContent(t, want, "original")
	if _, statErr := os.Lstat(source); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("source remains: %v", statErr)
	}
	sourceDir := filepath.Join(root, "disc")
	recoveryWrite(t, filepath.Join(sourceDir, "other.txt"), "directory content")
	got, err = filesystem.Recover(t.Context(), root, sourceDir)
	wantDir := filepath.Join(filepath.Dir(filepath.Dir(want)), "disc.1")
	if err != nil || got != wantDir {
		t.Fatalf("directory recovery = %q, %v; want %q", got, err, wantDir)
	}
	recoveryContent(t, filepath.Join(got, "other.txt"), "directory content")
}

func TestRecoverCollisionAtMoveDoesNotOverwrite(t *testing.T) {
	root, source, want := recoveryFixture(t)
	got, err := filesystem.RecoverWithRename(t.Context(), root, source, func(src, dst string) error {
		if dst == want {
			recoveryWrite(t, dst, "earlier recovery")
		}
		return filesystem.RenameNoReplace(src, dst)
	})
	wantSuffix := filepath.Join(filepath.Dir(want), "track.1.mp3")
	if err != nil || got != wantSuffix {
		t.Fatalf("Recover = %q, %v; want %q", got, err, wantSuffix)
	}
	recoveryContent(t, want, "earlier recovery")
	recoveryContent(t, got, "original")
}

func TestRecoverExhaustedNamesPreservesSource(t *testing.T) {
	root, source, want := recoveryFixture(t)
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	recoveryWrite(t, want, "earlier recovery")
	for n := 1; n <= 100; n++ {
		recoveryWrite(t, filepath.Join(filepath.Dir(want), fmt.Sprintf("track.%d.mp3", n)), "earlier recovery")
	}
	got, err := filesystem.Recover(t.Context(), root, source)
	if got != "" || !errors.Is(err, fs.ErrExist) {
		t.Fatalf("Recover = %q, %v; want name exhaustion", got, err)
	}
	recoveryContent(t, source, "original")
	recoveryContent(t, want, "earlier recovery")
}

func TestRecoverRefusesOutsideWork(t *testing.T) {
	root, source, _ := recoveryFixture(t)
	got, err := filesystem.Recover(t.Context(), filepath.Join(root, "disc"), root)
	if err == nil || got != "" {
		t.Fatalf("outside work accepted: %q, %v", got, err)
	}
	recoveryContent(t, source, "original")
}

func TestRecoverRefusesRecoverySymlink(t *testing.T) {
	root, source, _ := recoveryFixture(t)
	out := t.TempDir()
	if err := os.Symlink(out, filepath.Join(filepath.Dir(root), "Delete")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	got, err := filesystem.Recover(t.Context(), root, source)
	if err == nil || got != "" {
		t.Fatalf("symlink accepted: %q, %v", got, err)
	}
	recoveryContent(t, source, "original")
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != 0 {
		t.Fatalf("recovery escaped through symlink: %v, %v", entries, err)
	}
}

func TestRecoverRetriesLockedMoves(t *testing.T) {
	for _, succeeds := range []bool{true, false} {
		t.Run(strconv.FormatBool(succeeds), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				root, source, want := recoveryFixture(t)
				attempts := 0
				got, err := filesystem.RecoverWithRename(t.Context(), root, source, func(src, dst string) error {
					attempts++
					if succeeds && attempts == 3 {
						return filesystem.RenameNoReplace(src, dst)
					}
					return &os.LinkError{Op: "rename", Old: src, New: dst, Err: syscall.EBUSY}
				})
				if attempts != 3 {
					t.Fatalf("retry attempts = %d; want 3", attempts)
				}
				if succeeds {
					if err != nil || got != want {
						t.Fatalf("Recover = %q, %v", got, err)
					}
					recoveryContent(t, got, "original")
				} else {
					if !errors.Is(err, syscall.EBUSY) || got != "" {
						t.Fatalf("Recover = %q, %v; want locked failure", got, err)
					}
					recoveryContent(t, source, "original")
				}
			})
		})
	}
}

func TestRecoverPermissionFailureDoesNotRetry(t *testing.T) {
	root, source, _ := recoveryFixture(t)
	attempts := 0
	_, err := filesystem.RecoverWithRename(t.Context(), root, source, func(_, _ string) error {
		attempts++
		return fs.ErrPermission
	})
	if !errors.Is(err, fs.ErrPermission) || attempts != 1 {
		t.Fatalf("error = %v, attempts = %d; want immediate permission failure", err, attempts)
	}
	recoveryContent(t, source, "original")
}

func TestRecoverCancellationStopsLockedWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		root, source, _ := recoveryFixture(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		attempts := 0
		go func() {
			_, err := filesystem.RecoverWithRename(ctx, root, source, func(_, _ string) error {
				attempts++
				return syscall.EBUSY
			})
			done <- err
		}()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v; want canceled", err)
		}
		if attempts != 1 {
			t.Fatalf("attempts = %d; cancellation should stop before retry", attempts)
		}
		recoveryContent(t, source, "original")
	})
}
