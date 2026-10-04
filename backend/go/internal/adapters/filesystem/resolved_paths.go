package filesystem

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/onsei/organizer/backend/internal/fileops"
	"github.com/onsei/organizer/backend/internal/librarypath"
)

// IsResolvedWithinRoot reports whether candidate resolves to root or one of its
// descendants on the local filesystem. Both paths must exist. EvalSymlinks
// also resolves Windows junctions, preventing filesystem indirection from
// escaping a lexically valid library root.
func IsResolvedWithinRoot(root, candidate string) (bool, error) {
	if root == "" || candidate == "" {
		return false, nil
	}

	rootPath, err := filepath.Abs(filepath.FromSlash(librarypath.NormalizeToPOSIX(root)))
	if err != nil {
		return false, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		return false, err
	}

	candidatePath, err := filepath.Abs(filepath.FromSlash(librarypath.NormalizeToPOSIX(candidate)))
	if err != nil {
		return false, err
	}
	resolvedCandidate, err := filepath.EvalSymlinks(candidatePath)
	if err != nil {
		return false, err
	}

	// filepath.Rel compares path elements case-sensitively and
	// filepath.EvalSymlinks does not canonicalize letter case, so on Windows a
	// drive-letter or component case mismatch (C:/Music vs C:/MUSIC/album)
	// would produce a ..-prefixed result and falsely report the candidate as
	// outside the root. Fold case for the containment comparison only, so this
	// check agrees with librarypath.IsWithinRoot (which already folds Windows case).
	relRoot, relCandidate := resolvedRoot, resolvedCandidate
	if runtime.GOOS == "windows" {
		relRoot = strings.ToLower(resolvedRoot)
		relCandidate = strings.ToLower(resolvedCandidate)
	}
	relative, err := filepath.Rel(relRoot, relCandidate)
	if err != nil {
		return false, err
	}
	return relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative), nil
}

// ResolveMember applies file-management containment rules using the host filesystem.
func ResolveMember(rootPath, memberRel string) (string, error) {
	return fileops.ResolveMember(rootPath, memberRel, IsResolvedWithinRoot)
}

// ResolveMemberFile applies those same rules to one member-relative file.
func ResolveMemberFile(rootPath, memberRel, rel string) (string, error) {
	return fileops.ResolveMemberFile(rootPath, memberRel, rel, IsResolvedWithinRoot)
}
