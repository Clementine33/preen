package filesystem

import (
	"os"
	"path/filepath"
)

// ConversionHandles confines path resolution while giving each encode task
// separate seekable handles. O_EXCL refuses a pre-existing staging name.
type ConversionHandles struct{}

func (ConversionHandles) OpenSource(root, source string) (*os.File, error) {
	return openConversionFile(root, source, os.O_RDONLY, 0)
}

func (ConversionHandles) CreateStage(root, stage string) (*os.File, error) {
	return openConversionFile(root, stage, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
}

func openConversionFile(root, name string, flags int, mode os.FileMode) (*os.File, error) {
	directory, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	rel, err := filepath.Rel(root, name)
	if err != nil {
		return nil, err
	}
	return directory.OpenFile(rel, flags, mode)
}
