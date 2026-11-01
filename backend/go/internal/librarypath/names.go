package librarypath

import (
	"errors"
	"syscall"
	"unicode/utf16"
)

// ErrNameTooLong marks a path component refused because it does not fit the
// platform's name limit, before or after the filesystem was asked.
var ErrNameTooLong = errors.New("name too long")

// IsNameTooLong reports a name refusal: ErrNameTooLong, ENAMETOOLONG, or
// Windows ERROR_FILENAME_EXCED_RANGE (206; syscall.Errno exists on every host).
func IsNameTooLong(err error) bool {
	return errors.Is(err, ErrNameTooLong) || errors.Is(err, syscall.ENAMETOOLONG) ||
		errors.Is(err, syscall.Errno(206))
}

// maxNameUnits is the per-component limit: UTF-8 bytes on POSIX filesystems,
// UTF-16 code units on Windows.
const maxNameUnits = 255

// NameFits reports whether name fits as one path component under root. The
// platform is read from root's syntax like the rest of this package: a drive
// or device path counts UTF-16 code units, a UNC path must also fit in UTF-8
// bytes (the server behind a share commonly counts bytes), and any other path
// counts UTF-8 bytes.
func NameFits(root, name string) bool {
	bytesFit := len(name) <= maxNameUnits
	switch {
	case IsWindowsUNCPath(root):
		return bytesFit && utf16Len(name) <= maxNameUnits
	case IsWindowsCaseInsensitivePath(root):
		return utf16Len(name) <= maxNameUnits
	default:
		return bytesFit
	}
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}
