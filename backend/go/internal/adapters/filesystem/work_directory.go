package filesystem

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// CheckWorkDirectory checks the frozen basename without reading its children or
// capturing a physical identity. Same-path replacement directories are allowed.
func CheckWorkDirectory(path string) error {
	native := filepath.FromSlash(path)
	info, err := os.Lstat(native)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Name() != filepath.Base(native) {
		return fmt.Errorf("work directory missing or renamed (%s): %w", path, fs.ErrNotExist)
	}
	return nil
}
