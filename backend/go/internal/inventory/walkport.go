package inventory

import "context"

// DirEntry is one directory or file a traversal emitted: the observed fact the
// scan pipeline stages, before a session and a format are attached to it.
type DirEntry struct {
	Path         string
	ParentPath   string
	Name         string
	IsDir        bool
	IsWorkRoot   bool
	Size         int64
	Mtime        int64
	FilesCarried int
	Carried      bool
}

// ReuseDirectory decides whether a directory's direct files can be carried over.
// The traversal supplies filesystem facts; the pipeline supplies inventory policy.
type ReuseDirectory func(path string, mtime int64) (DirectoryReuse, bool)

// DirectoryReuse retains direct file counts and, for nonleaves, entry names and types.
type DirectoryReuse struct {
	Mtime     int64
	FileCount int
	Entries   map[string]bool // nil for leaves; otherwise name → isDir
}

// WalkRoot is one full-root traversal strategy: bounded work scanning
// under the root, emitting every directory and each file it observes. Eligible
// directories carry their direct inventory files without reading them. The pipeline
// reads the filesystem through this port, so the traversal lives in the
// filesystem adapter and the pipeline stays testable without one.
type WalkRoot func(
	ctx context.Context,
	rootPath, basePath string,
	dirConcurrency int,
	emit func(DirEntry) error,
	reuse ...ReuseDirectory,
) error

// WalkMember is the single-enumerator traversal of one member directory: the
// refresh path reads one subtree with one enumerator, which is what makes a
// page refresh cheap next to a full scan.
type WalkMember func(ctx context.Context, folderPath, basePath string, emit func(DirEntry) error) error
