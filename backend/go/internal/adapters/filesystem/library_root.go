package filesystem

import (
	"errors"
	"os"
)

// CheckLibraryRoot reports whether root is an existing directory.
func CheckLibraryRoot(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("not a directory")
	}
	return nil
}
