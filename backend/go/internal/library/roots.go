package library

import "github.com/Clementine33/preen/backend/internal/librarypath"

// CheckRootOverlap rejects strict containment; canonical duplicates retain their separate identity error.
func CheckRootOverlap(root, other string) error {
	if librarypath.RootPathKey(root) != librarypath.RootPathKey(other) &&
		(librarypath.IsWithinRoot(root, other) || librarypath.IsWithinRoot(other, root)) {
		return ErrLibraryRootOverlap
	}
	return nil
}
