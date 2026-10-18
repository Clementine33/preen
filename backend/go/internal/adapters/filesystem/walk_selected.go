package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/Clementine33/preen/backend/internal/fileops"

	"golang.org/x/sync/errgroup"

	"github.com/Clementine33/preen/backend/internal/inventory"
)

// WalkSelectedEntriesParallel bounds concurrency between selected folders;
// each folder's descendants use the existing sequential walker. Callbacks for
// distinct folders may run concurrently. Empty input performs no filesystem IO.
func WalkSelectedEntriesParallel(
	ctx context.Context,
	folders []string,
	base string,
	workers int,
	emit func(string, inventory.DirEntry) error,
) error {
	if workers <= 0 {
		workers = inventory.DefaultScanWorkers
	}
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(min(workers, 32))
	seen := []string{}
	for _, folder := range folders {
		folder = filepath.Clean(filepath.FromSlash(folder))
		if slices.Contains(seen, folder) {
			continue
		}
		seen = append(seen, folder)
		g.Go(func() error {
			rel, relErr := filepath.Rel(filepath.FromSlash(base), folder)
			if relErr != nil {
				return relErr
			}
			resolved, resolveErr := ResolveMember(base, filepath.ToSlash(rel))
			if resolveErr != nil {
				if pathErr, ok := errors.AsType[*fileops.PathError](
					resolveErr,
				); ok &&
					pathErr.Code == fileops.CodeMemberMissing {
					return fmt.Errorf("walk %s: %w", folder, fs.ErrNotExist)
				}
				return resolveErr
			}
			err := WalkFolderEntries(
				ctx,
				resolved,
				filepath.FromSlash(base),
				func(entry inventory.DirEntry) error { return emit(filepath.ToSlash(folder), entry) },
			)
			if err != nil {
				return fmt.Errorf("walk %s: %w", folder, err)
			}
			return nil
		})
	}
	return g.Wait()
}
