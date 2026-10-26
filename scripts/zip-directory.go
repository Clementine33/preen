// Command zip-directory archives a prepared portable layout using the pinned Go toolchain.
package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func zipDirectory(directory, destination string) (err error) {
	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(output)
	defer func() { err = errors.Join(err, archive.Close(), output.Close()) }()
	return filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular portable input: %s", path)
		}
		name, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		writer, err := archive.Create(filepath.ToSlash(name))
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, input)
		return errors.Join(copyErr, input.Close())
	})
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: zip-directory DIRECTORY OUTPUT.zip")
		os.Exit(1)
	}
	if err := zipDirectory(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
