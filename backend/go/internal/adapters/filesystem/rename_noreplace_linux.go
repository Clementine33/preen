//go:build linux

package filesystem

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// RenameNoReplace renames src to dst and fails when dst exists. Linux gets the
// real primitive: renameat2(RENAME_NOREPLACE) makes the existence check and the
// move one atomic step, so a destination that appears between them is refused
// by the kernel instead of being overwritten.
//
// A filesystem that does not implement the flag (EINVAL, ENOSYS, EOPNOTSUPP)
// falls back to a check immediately before os.Rename; external writers may
// still race with this portable fallback.
func RenameNoReplace(src, dst string) error {
	err := unix.Renameat2(unix.AT_FDCWD, src, unix.AT_FDCWD, dst, unix.RENAME_NOREPLACE)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, unix.EINVAL), errors.Is(err, unix.ENOSYS), errors.Is(err, unix.EOPNOTSUPP):
		return renameChecked(src, dst)
	default:
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: err}
	}
}
