package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/onsei/organizer/backend/internal/librarypath"
)

// Recover moves one file or directory from a work root into
// <work parent>/Delete/<work name>/<relative path>. Existing recovery media is
// never overwritten: collisions advance through <stem>.N<ext>, up to N=100.
// Locked moves are retried at most three times; cancellation stops waiting.
// The returned native absolute path is the only location callers report.
func Recover(ctx context.Context, workRoot, source string) (string, error) {
	return recoverFile(ctx, workRoot, source, RenameNoReplace)
}

func recoverFile(
	ctx context.Context, workRoot, source string, rename func(string, string) error,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	parent, relDir, err := recoveryDirectory(workRoot, source)
	if err != nil {
		return "", err
	}
	dir, err := createRecoveryDirectory(parent, relDir)
	if err != nil {
		return "", err
	}
	base := filepath.Base(source)
	const maxNameAttempts = 100
	for attempt := 0; attempt <= maxNameAttempts; attempt++ {
		name := base
		if attempt > 0 {
			ext := filepath.Ext(base)
			name = fmt.Sprintf("%s.%d%s", strings.TrimSuffix(base, ext), attempt, ext)
		}
		target := filepath.Join(dir, name)
		err := renameRecovery(ctx, source, target, rename)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("move to recovery: %w", err)
		}
		return target, nil
	}
	return "", fmt.Errorf("no free recovery name for %s: %w", base, fs.ErrExist)
}

// createRecoveryDirectory refuses links before descending, so a pre-existing
// recovery directory cannot redirect creation or moves outside the work parent.
func createRecoveryDirectory(parent, rel string) (string, error) {
	current := parent
	for part := range strings.SplitSeq(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		if err := os.Mkdir(current, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("create recovery directory: %w", err)
		}
		info, err := os.Lstat(current)
		if err != nil {
			return "", fmt.Errorf("inspect recovery directory: %w", err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("recovery path is not a real directory: %s", current)
		}
	}
	return current, nil
}

func renameRecovery(ctx context.Context, source, target string, rename func(string, string) error) error {
	const maxAttempts = 3
	for attempt := range maxAttempts {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := rename(source, target)
		if err == nil || !recoveryMoveLocked(err) || attempt == maxAttempts-1 {
			return err
		}
		timer := time.NewTimer(time.Duration(1<<attempt) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

func recoveryMoveLocked(err error) bool {
	if _, incomplete := errors.AsType[*recoveryMoveIncompleteError](err); incomplete {
		return false
	}
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrExist) {
		return false
	}
	if errors.Is(err, syscall.EBUSY) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "locked") || strings.Contains(message, "being used") ||
		strings.Contains(message, "accessed by another process")
}

// recoveryDirectory shares the established layout and UNC truncation with probes.
func recoveryDirectory(workRoot, source string) (string, string, error) {
	workRoot = filepath.Clean(workRoot)
	source = filepath.Clean(source)
	if !filepath.IsAbs(workRoot) || !filepath.IsAbs(source) {
		return "", "", fmt.Errorf("recovery paths must be absolute")
	}
	rel, err := filepath.Rel(workRoot, source)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("recover %s: not inside the work root", source)
	}
	parent := filepath.Dir(workRoot)
	relDir := filepath.Join(filepath.Base(workRoot), filepath.Dir(rel))
	if librarypath.IsWindowsUNCPath(parent) {
		relDir = TruncatePathComponentsToBytes(relDir, 214)
	}
	return parent, filepath.Join(librarypath.RecoveryDirName, relDir), nil
}
