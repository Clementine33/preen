package filesystem

import (
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// TruncatePathComponentsToBytes keeps UNC recovery names within their byte budget
// without splitting a UTF-8 character. It operates on host-native separators.
func TruncatePathComponentsToBytes(path string, maxBytes int) string {
	if maxBytes <= 0 || path == "" || path == "." {
		return path
	}

	sep := string(filepath.Separator)
	parts := strings.Split(path, sep)
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			continue
		}
		parts[i] = truncateUTF8ToMaxBytes(part, maxBytes)
	}
	return strings.Join(parts, sep)
}

func truncateUTF8ToMaxBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	byteCount := 0
	cut := 0
	for _, r := range s {
		runeBytes := utf8.RuneLen(r)
		if byteCount+runeBytes > maxBytes {
			break
		}
		byteCount += runeBytes
		cut += runeBytes
	}

	return s[:cut]
}
