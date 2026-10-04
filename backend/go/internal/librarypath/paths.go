package librarypath

import (
	"path"
	"strings"
)

func NormalizeToPOSIX(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

// RecoveryDirName is the reserved soft-deletion directory beside a work root.
// Removed media is preserved at <work parent>/Delete/<work name>/<relative path>.
const RecoveryDirName = "Delete"

// IsRecoveryRelPath reports whether a library-relative path enters a recovery
// directory at any depth. Windows path components compare case-insensitively.
func IsRecoveryRelPath(rootPath, rel string) bool {
	for name := range strings.SplitSeq(NormalizeToPOSIX(rel), "/") {
		if name == RecoveryDirName ||
			(IsWindowsCaseInsensitivePath(rootPath) && strings.EqualFold(name, RecoveryDirName)) {
			return true
		}
	}
	return false
}

// IsWindowsCaseInsensitivePath reports whether a path is syntactically a
// Windows path (drive, UNC or device form), where component names compare
// case-insensitively regardless of the host the backend runs on.
func IsWindowsCaseInsensitivePath(p string) bool {
	normalized := NormalizeToPOSIX(p)
	return IsWindowsUNCPath(p) || isWindowsDrivePath(normalized) || strings.HasPrefix(normalized, "//?/")
}

// IsWithinRoot reports whether candidate is root itself or one of its
// descendants. Paths are compared in their persisted POSIX form, with lexical
// cleaning and component boundaries so sibling prefixes and parent traversal
// cannot escape the root. Windows drive and UNC paths compare case-insensitively.
func IsWithinRoot(root, candidate string) bool {
	if root == "" || candidate == "" {
		return false
	}

	normalizedRoot := NormalizeToPOSIX(root)
	windowsPath := IsWindowsUNCPath(root) || isWindowsDrivePath(normalizedRoot) ||
		strings.HasPrefix(normalizedRoot, "//?/")
	rootIsDrive := isWindowsDriveRoot(normalizedRoot)
	normalizedRoot = path.Clean(normalizedRoot)
	if rootIsDrive {
		normalizedRoot += "/"
	}
	normalizedCandidate := NormalizeToPOSIX(candidate)
	candidateIsDrive := isWindowsDriveRoot(normalizedCandidate)
	normalizedCandidate = path.Clean(normalizedCandidate)
	if candidateIsDrive {
		normalizedCandidate += "/"
	}

	if windowsPath {
		normalizedRoot = strings.ToLower(normalizedRoot)
		normalizedCandidate = strings.ToLower(normalizedCandidate)
	}
	if normalizedCandidate == normalizedRoot {
		return true
	}
	if normalizedRoot == "/" {
		return strings.HasPrefix(normalizedCandidate, "/")
	}
	return strings.HasPrefix(normalizedCandidate, strings.TrimSuffix(normalizedRoot, "/")+"/")
}

// RelPath validates a library-relative path and returns its cleaned POSIX
// form. A relative path is a plain descendant chain: it never names the root
// itself, and it may not be absolute, carry a drive or device prefix, contain a
// backslash (the separator of a foreign filesystem) or NUL byte, or hold an
// empty, "." or ".." segment. Everything the caller can reach through the
// result is therefore inside the root it will be joined to, which is what makes
// a traversal attempt a rejection instead of a containment question.
func RelPath(rel string) (string, bool) {
	if rel == "" || strings.ContainsRune(rel, 0) {
		return "", false
	}
	// The separator of a foreign filesystem is refused in its own form, before
	// normalization would turn it into a legal-looking separator.
	if strings.Contains(rel, `\`) {
		return "", false
	}
	normalized := NormalizeToPOSIX(rel)
	if strings.HasPrefix(normalized, "/") {
		return "", false
	}
	// A drive- or device-prefixed path is absolute wherever it is read, so it
	// is never a relative one.
	if isWindowsDrivePath(normalized) {
		return "", false
	}
	segments := strings.Split(normalized, "/")
	for _, seg := range segments {
		if seg == "" || seg == "." || seg == ".." {
			return "", false
		}
	}
	cleaned := path.Join(segments...)
	if cleaned == "" || cleaned == "." || strings.HasPrefix(cleaned, "/") {
		return "", false
	}
	return cleaned, true
}

// JoinRel resolves a validated relative path against a root. The caller must
// have validated the relative path with RelPath; this joins in the persisted
// POSIX form so the result matches the paths the scanner stores.
func JoinRel(rootPath, relPath string) string {
	return CleanRootPath(strings.TrimRight(NormalizeToPOSIX(rootPath), "/") + "/" + relPath)
}

// CleanRootPath normalizes a library root for storage: backslashes become
// forward slashes, `.`/`..` elements and repeated separators are collapsed,
// and root forms (`/`, `C:/`, `//server/share`) keep their trailing structure.
// The result is the display/storage form; use RootPathKey for identity.
func CleanRootPath(p string) string {
	p = NormalizeToPOSIX(p)
	if isWindowsDriveRoot(p) {
		return p
	}
	if after, ok := strings.CutPrefix(p, "//?"); ok {
		rest := after
		if rest == "" {
			return "//?"
		}
		cleaned := path.Clean(rest)
		if cleaned == "." || cleaned == "/" {
			return "//?"
		}
		return "//?" + "/" + strings.TrimPrefix(cleaned, "/")
	}
	if after, ok := strings.CutPrefix(p, "//"); ok {
		rest := after
		if rest == "" {
			return "//"
		}
		cleaned := path.Clean(rest)
		if cleaned == "." || cleaned == "/" {
			return "//"
		}
		return "//" + cleaned
	}
	return path.Clean(p)
}

// RootPathKey returns the canonical identity key for a library root: the
// cleaned root, with letter case folded only for syntactically Windows-style
// paths (drive, UNC, and device paths). POSIX paths stay case-sensitive,
// matching their filesystems. The decision is based on the path syntax, not
// the host OS, so `C:/Music` and `c:/music` collide wherever the backend runs.
func RootPathKey(p string) string {
	cleaned := CleanRootPath(p)
	if IsWindowsCaseInsensitivePath(p) {
		return strings.ToLower(cleaned)
	}
	return cleaned
}

func isWindowsDrivePath(p string) bool {
	return len(p) >= 2 && p[1] == ':' && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z'))
}

func isWindowsDriveRoot(p string) bool {
	return len(p) == 3 && isWindowsDrivePath(p) && p[2] == '/'
}

func IsWindowsUNCPath(path string) bool {
	normalized := strings.ReplaceAll(path, "/", "\\")
	return strings.HasPrefix(normalized, `\\?\UNC\`) ||
		(strings.HasPrefix(normalized, `\\`) && !strings.HasPrefix(normalized, `\\?\`))
}
