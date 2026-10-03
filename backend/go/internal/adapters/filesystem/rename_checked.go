//go:build !windows

package filesystem

import (
	"errors"
	"io/fs"
	"os"
)

// renameChecked is the portable fallback: refuse an existing destination right
// before the rename when the filesystem has no atomic no-replace operation.
func renameChecked(src, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: fs.ErrExist}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: err}
	}
	return os.Rename(src, dst)
}
