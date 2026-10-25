package filesystem

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/librarypath"
	"github.com/Clementine33/preen/backend/internal/works"
)

// dirEntryInfoFunc is a seam for testing Info() error paths.
var dirEntryInfoFunc func(fs.DirEntry) (fs.FileInfo, error)

// WalkRootEntriesParallel discovers work roots while bounded workers scan each
// work sequentially. Classification directories are traversed by the discoverer.
// Observed entries are emitted once; reusable directories carry their direct files.
// The library root is observed as a classification directory; recovery subtrees are excluded.
func WalkRootEntriesParallel(
	ctx context.Context,
	rootPath, basePath string,
	workers int,
	emit func(inventory.DirEntry) error,
	reuse ...inventory.ReuseDirectory,
) error {
	if workers <= 0 {
		workers = inventory.DefaultScanWorkers
	}
	workers = min(workers, 32)
	group, ctx := errgroup.WithContext(ctx)
	jobs := make(chan inventory.DirEntry, 2*workers)
	var carry inventory.ReuseDirectory
	if len(reuse) > 0 {
		carry = reuse[0]
	}
	group.Go(func() error {
		defer close(jobs)
		info, err := os.Stat(rootPath)
		if err != nil {
			return err
		}
		root := inventory.DirEntry{
			Path: rootPath, ParentPath: filepath.Dir(rootPath), Name: filepath.Base(rootPath),
			IsDir: true, Mtime: info.ModTime().Unix(),
		}
		return walkEntries(ctx, rootPath, basePath, emit, jobs, carry, &root)
	})
	for range workers {
		group.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case folder, ok := <-jobs:
					if !ok {
						return ctx.Err()
					}
					if err := walkEntries(ctx, folder.Path, basePath, emit, nil, carry, &folder); err != nil {
						return err
					}
				}
			}
		})
	}
	return group.Wait()
}

// WalkFolderEntries streams a single work's descendants in name order without
// recognizing additional works. The work root's inventory marker is preserved.
func WalkFolderEntries(ctx context.Context, folderPath, basePath string, emit func(inventory.DirEntry) error) error {
	rel, err := filepath.Rel(basePath, folderPath)
	if err != nil {
		return err
	}
	if librarypath.IsRecoveryRelPath(basePath, filepath.ToSlash(rel)) {
		return ctx.Err()
	}
	return walkEntries(ctx, folderPath, basePath, emit, nil, nil, nil)
}

func walkEntries(
	ctx context.Context,
	currentPath, basePath string,
	emit func(inventory.DirEntry) error,
	jobs chan<- inventory.DirEntry,
	reuse inventory.ReuseDirectory,
	directory *inventory.DirEntry,
) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	entries, err := listObservedDirectory(currentPath, basePath, emit, reuse, directory)
	if err != nil {
		return err
	}
	for _, d := range entries {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		entryPath := filepath.Join(currentPath, d.Name())
		// Soft-deleted media is outside the inventory; skip before stat or descent.
		if d.IsDir() && librarypath.IsRecoveryRelPath(basePath, d.Name()) {
			continue
		}
		if !d.IsDir() && directory != nil && directory.Carried {
			continue
		}
		var info fs.FileInfo
		if dirEntryInfoFunc != nil {
			info, err = dirEntryInfoFunc(d)
		} else {
			info, err = d.Info()
		}
		if err != nil {
			return err
		}
		isWorkRoot := jobs != nil && d.IsDir() &&
			works.CodesInDirName(d.Name()).Kind == works.DirCodeSingle
		entry := inventory.DirEntry{
			Path: entryPath, ParentPath: filepath.Dir(entryPath), Name: d.Name(),
			IsDir: d.IsDir(), IsWorkRoot: isWorkRoot, Size: info.Size(), Mtime: info.ModTime().Unix(),
		}
		if !d.IsDir() {
			if err := emit(entry); err != nil {
				return err
			}
			continue
		}
		if isWorkRoot && reuse != nil {
			if retained, ok := reuse(entryPath, entry.Mtime); ok && retained.Entries == nil {
				entry.Carried, entry.FilesCarried = true, retained.FileCount
				if err := emit(entry); err != nil {
					return err
				}
				continue
			}
		}
		if isWorkRoot {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case jobs <- entry:
			}
		} else if err := walkEntries(ctx, entryPath, basePath, emit, jobs, reuse, &entry); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// listObservedDirectory pairs the directory observation with its complete listing or carried files.
func listObservedDirectory(
	currentPath, basePath string,
	emit func(inventory.DirEntry) error,
	reuse inventory.ReuseDirectory,
	directory *inventory.DirEntry,
) ([]fs.DirEntry, error) {
	var retained inventory.DirectoryReuse
	if directory != nil && reuse != nil {
		retained, directory.Carried = reuse(currentPath, directory.Mtime)
		if directory.Carried && retained.Entries == nil {
			directory.FilesCarried = retained.FileCount
			return nil, emit(*directory)
		}
	}
	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return nil, err
	}
	if directory != nil {
		if directory.Carried {
			directory.Carried = matchingEntries(entries, retained.Entries, basePath)
			if directory.Carried {
				directory.FilesCarried = retained.FileCount
			}
		}
		if err = emit(*directory); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func matchingEntries(entries []fs.DirEntry, retained map[string]bool, basePath string) bool {
	count := 0
	for _, entry := range entries {
		if entry.IsDir() && librarypath.IsRecoveryRelPath(basePath, entry.Name()) {
			continue
		}
		isDir, ok := retained[entry.Name()]
		if !ok || isDir != entry.IsDir() {
			return false
		}
		count++
	}
	return count == len(retained)
}

// Allowed audio extensions.
var AllowedExtensions = map[string]bool{
	".wav":  true,
	".flac": true,
	".mp3":  true,
	".aac":  true,
	".m4a":  true,
	".opus": true,
}

// FileEntry represents a scanned file.
type FileEntry struct {
	Path      string
	PathPosix string
	FileSize  int64
}

// ScanDirectory scans a directory for audio files.
func ScanDirectory(rootPath string) ([]FileEntry, error) {
	var entries []FileEntry

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Check extension
		ext := filepath.Ext(path)
		if !AllowedExtensions[ext] {
			return nil
		}

		// Get file info
		info, err := d.Info()
		if err != nil {
			return nil // Skip files we can't stat
		}

		// Convert path to POSIX format
		posixPath := filepath.ToSlash(path)

		entries = append(entries, FileEntry{
			Path:      path,
			PathPosix: posixPath,
			FileSize:  info.Size(),
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return entries, nil
}
