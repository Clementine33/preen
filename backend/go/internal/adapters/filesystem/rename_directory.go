package filesystem

import (
	"context"
	"os"
	"path/filepath"

	"github.com/onsei/organizer/backend/internal/dirrename"
)

// ReadRenameDirectory lists immediate entries without traversing symlinks.
func ReadRenameDirectory(ctx context.Context, root string) (dirrename.Directory, error) {
	entries, err := os.ReadDir(filepath.FromSlash(root))
	if err != nil {
		return nil, err
	}
	out := make(dirrename.Directory, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		out[entry.Name()] = dirrename.Entry{
			Directory: info.IsDir(),
			Symlink:   info.Mode()&os.ModeSymlink != 0,
			Modified:  info.ModTime().UnixNano(),
		}
	}
	return out, nil
}
