package filesystem

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

// CoverFiles confines cover publication to one checked library member.
type CoverFiles struct{}

func openCoverRoot(t coverdelivery.Target) (*os.Root, error) {
	dir, err := ResolveMember(t.LibraryRoot, t.RelPath)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	if err := CheckWorkDirectory(dir); err != nil {
		root.Close()
		return nil, coverdelivery.Failure("INPUT_CHANGED", err)
	}
	return root, nil
}
func coverName(name string) bool {
	switch name {
	case "cover.webp", "cover.jpg", "cover.jpeg", "cover.png", "cover.gif":
		return true
	}
	return false
}
func digest(data []byte) string { v := sha256.Sum256(data); return hex.EncodeToString(v[:]) }
func readImage(root *os.Root, name string) (*coverdelivery.Existing, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, coverdelivery.Failure("COVER_CONFLICT", fmt.Errorf("cover is not a regular file"))
	}
	if info.Size() > coverdelivery.MaxBytes {
		return nil, coverdelivery.Failure("INVALID_IMAGE", fmt.Errorf("image exceeds limit"))
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, coverdelivery.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	ext, _, err := coverdelivery.Inspect(data)
	if err != nil {
		return nil, err
	}
	return &coverdelivery.Existing{
		Name: name, Data: data, Extension: ext, Digest: digest(data),
		File: inventory.InventoryFile{Size: info.Size(), Mtime: info.ModTime().Unix()},
	}, nil
}
func existingCover(root *os.Root) (*coverdelivery.Existing, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	var found *coverdelivery.Existing
	for _, entry := range entries {
		if !coverName(entry.Name()) {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			return nil, e
		}
		if !info.Mode().IsRegular() {
			return nil, coverdelivery.Failure("COVER_CONFLICT", fmt.Errorf("cover path is not a regular file"))
		}
		image, e := readImage(root, entry.Name())
		if e != nil {
			if coverdelivery.Code(e) == "INVALID_IMAGE" {
				continue
			}
			return nil, e
		}
		if found != nil {
			return nil, coverdelivery.Failure("COVER_CONFLICT", fmt.Errorf("multiple valid covers"))
		}
		found = image
	}
	return found, nil
}
func (CoverFiles) Existing(t coverdelivery.Target) (*coverdelivery.Existing, error) {
	root, err := openCoverRoot(t)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	image, err := existingCover(root)
	if image != nil {
		image.File.Path = filepath.ToSlash(filepath.Join(t.LibraryRoot, t.RelPath, image.Name))
		_, pending := root.Lstat(coverJournal)
		image.NeedsRecovery = !os.IsNotExist(pending)
	}
	return image, err
}

const coverJournal = ".onsei-cover-publish.json"

type coverPublication struct {
	Name   string   `json:"name"`
	Temp   string   `json:"temp"`
	Digest string   `json:"digest"`
	Old    []string `json:"old"`
}

func prepareCoverPublication(root *os.Root, name, temp, sum string) (coverPublication, error) {
	dir, err := root.Open(".")
	if err != nil {
		return coverPublication{}, err
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return coverPublication{}, err
	}
	journal := coverPublication{Name: name, Temp: temp, Digest: sum, Old: []string{}}
	for _, entry := range entries {
		if coverName(entry.Name()) {
			info, e := entry.Info()
			if e != nil {
				return coverPublication{}, e
			}
			if !info.Mode().IsRegular() {
				return coverPublication{}, coverdelivery.Failure(
					"COVER_CONFLICT",
					fmt.Errorf("cover path is not a regular file"),
				)
			}
			if entry.Name() != name {
				journal.Old = append(journal.Old, entry.Name())
			}
		}
	}
	return journal, nil
}
