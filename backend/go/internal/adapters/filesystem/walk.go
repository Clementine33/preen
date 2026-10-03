package filesystem

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/pathnorm"
	"github.com/onsei/organizer/backend/internal/works"
)

// dirEntryInfoFunc is a seam for testing Info() error paths.
var dirEntryInfoFunc func(fs.DirEntry) (fs.FileInfo, error)

// WalkRootEntriesParallel discovers work roots while bounded workers scan each
// work sequentially. Classification directories are traversed by the discoverer.
// Every entry is emitted once; the library root and its recovery subtree are excluded.
func WalkRootEntriesParallel(
	ctx context.Context,
	rootPath, basePath string,
	workers int,
	emit func(inventory.DirEntry) error,
) error {
	if workers <= 0 {
		workers = inventory.DefaultScanWorkers
	}
	workers = min(workers, 32)
	group, ctx := errgroup.WithContext(ctx)
	jobs := make(chan string, 2*workers)
	group.Go(func() error {
		defer close(jobs)
		return walkEntries(ctx, rootPath, basePath, emit, jobs)
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
					if err := WalkFolderEntries(ctx, folder, basePath, emit); err != nil {
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
	if pathnorm.IsRecoveryRelPath(basePath, filepath.ToSlash(rel)) {
		return ctx.Err()
	}
	return walkEntries(ctx, folderPath, basePath, emit, nil)
}

func walkEntries(
	ctx context.Context,
	currentPath, basePath string,
	emit func(inventory.DirEntry) error,
	jobs chan<- string,
) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return err
	}
	for _, d := range entries {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		entryPath := filepath.Join(currentPath, d.Name())
		// Soft-deleted media is outside the inventory; skip before stat or descent.
		if d.IsDir() && pathnorm.IsRecoveryRelPath(basePath, d.Name()) {
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
		if err := emit(entry); err != nil {
			return err
		}
		if !d.IsDir() {
			continue
		}
		if isWorkRoot {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case jobs <- entryPath:
			}
		} else if err := walkEntries(ctx, entryPath, basePath, emit, jobs); err != nil {
			return err
		}
	}
	return ctx.Err()
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
