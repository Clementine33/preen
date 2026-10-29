package library

import "time"

// Store is the library storage this package needs: exactly the operations the
// use cases below call, no per-table repository and no read the library does
// not make. The SQLite adapter satisfies it with its one concrete Repository.
type Store interface {
	// CreateLibrary atomically checks disjoint root ownership and inserts a library.
	CreateLibrary(name, rootPath string) (*Library, error)
	// ListLibraries returns every library, newest first.
	ListLibraries() ([]*Library, error)
	// GetLibrary loads one library by id.
	GetLibrary(id string) (*Library, error)
	// UpdateLibrary applies partial fields and atomically rejects a root change that overlaps another library.
	UpdateLibrary(id string, name, rootPath *string, storageClass *StorageClass) (*Library, error)
	// DeleteLibrary removes the library and hides its record and sessions atomically.
	DeleteLibrary(id string) error
	// UpdateLibraryScanState records the outcome of a scan of this library.
	UpdateLibraryScanState(libraryID, status, message string, at time.Time) error
}
