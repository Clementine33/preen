//go:build windows

package filesystem

import (
	"path/filepath"
	"strings"
)

// longPathThreshold mirrors the directory limit os.fixLongPath uses: below it
// the Win32 path is still accepted without the extended-length prefix.
const longPathThreshold = 248

// fixLongPath makes an absolute path usable by raw Win32 calls the way the os
// package does internally: it cleans the path and adds the `\\?\` (or
// `\\?\UNC\`) prefix once it is long enough. Relative and already-prefixed
// paths are returned unchanged.
func fixLongPath(path string) string {
	if len(path) < longPathThreshold || strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return path
	}
	if !filepath.IsAbs(path) {
		return path
	}
	path = filepath.Clean(path)
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + path[2:]
	}
	return `\\?\` + path
}

// trimLongPathPrefix undoes fixLongPath so identities compare equal for short
// and long spellings of the same volume.
func trimLongPathPrefix(path string) string {
	if rest, ok := strings.CutPrefix(path, `\\?\UNC\`); ok {
		return `\\` + rest
	}
	return strings.TrimPrefix(path, `\\?\`)
}
