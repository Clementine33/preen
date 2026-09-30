package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/onsei/organizer/backend/internal/covers"
	"github.com/onsei/organizer/backend/internal/fileops"
)

// CoverFiles confines cover publication to one checked library member.
type CoverFiles struct{}

func CoverDirectoryIdentity(path string) (string, error) {
	info, err := os.Lstat(filepath.FromSlash(path))
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("not a real directory")
	}
	return coverIdentity(info, filepath.FromSlash(path))
}
func openCoverRoot(t covers.Target) (*os.Root, error) {
	dir, err := fileops.ResolveMember(t.LibraryRoot, t.RelPath)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	id, err := CoverDirectoryIdentity(dir)
	if err != nil || t.Identity != "" && id != t.Identity {
		root.Close()
		return nil, covers.Failure("INPUT_CHANGED", fmt.Errorf("directory replaced"))
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
func readImage(root *os.Root, name string) (*covers.Existing, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, covers.Failure("COVER_CONFLICT", fmt.Errorf("cover is not a regular file"))
	}
	if info.Size() > covers.MaxBytes {
		return nil, covers.Failure("INVALID_IMAGE", fmt.Errorf("image exceeds limit"))
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, covers.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	ext, _, err := covers.Inspect(data)
	if err != nil {
		return nil, err
	}
	return &covers.Existing{Name: name, Data: data, Extension: ext, Digest: digest(data)}, nil
}
func existingCover(root *os.Root) (*covers.Existing, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	var found *covers.Existing
	for _, entry := range entries {
		if !coverName(entry.Name()) {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			return nil, e
		}
		if !info.Mode().IsRegular() {
			return nil, covers.Failure("COVER_CONFLICT", fmt.Errorf("cover path is not a regular file"))
		}
		image, e := readImage(root, entry.Name())
		if e != nil {
			if covers.Code(e) == "INVALID_IMAGE" {
				continue
			}
			return nil, e
		}
		if found != nil {
			return nil, covers.Failure("COVER_CONFLICT", fmt.Errorf("multiple valid covers"))
		}
		found = image
	}
	return found, nil
}
func (CoverFiles) Existing(t covers.Target) (*covers.Existing, error) {
	root, err := openCoverRoot(t)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	image, err := existingCover(root)
	if image != nil {
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

// recoverCover cleans old extensions only if the new bytes really landed.
func recoverCover(root *os.Root) error {
	info, err := root.Lstat(coverJournal)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 8192 {
		return covers.Failure("COVER_CONFLICT", fmt.Errorf("invalid publication journal"))
	}
	raw, err := root.ReadFile(coverJournal)
	if err != nil {
		return err
	}
	var j coverPublication
	if err = json.Unmarshal(raw, &j); err != nil {
		return err
	}
	if !coverName(j.Name) || !strings.HasPrefix(j.Temp, ".onsei-cover-") || !strings.HasSuffix(j.Temp, ".tmp") ||
		filepath.Base(j.Temp) != j.Temp {
		return fmt.Errorf("invalid cover journal paths")
	}
	image, e := readImage(root, j.Name)
	if e == nil && image.Digest == j.Digest {
		for _, old := range j.Old {
			if !coverName(old) || old == j.Name {
				continue
			}
			info, statErr := root.Lstat(old)
			if os.IsNotExist(statErr) {
				continue
			}
			if statErr != nil {
				return statErr
			}
			if !info.Mode().IsRegular() {
				return covers.Failure("COVER_CONFLICT", fmt.Errorf("old cover is no longer a regular file"))
			}
			if e = root.Remove(old); e != nil && !os.IsNotExist(e) {
				return e
			}
		}
	}
	if e = root.Remove(j.Temp); e != nil && !os.IsNotExist(e) {
		return e
	}
	return root.Remove(coverJournal)
}
func (CoverFiles) Publish(ctx context.Context, t covers.Target, data []byte, refresh bool) (covers.Publication, error) {
	root, err := openCoverRoot(t)
	if err != nil {
		return covers.Publication{}, err
	}
	defer root.Close()
	if err = recoverCover(root); err != nil {
		return covers.Publication{}, err
	}
	if !refresh {
		old, e := existingCover(root)
		if e != nil {
			return covers.Publication{}, e
		}
		if old != nil {
			return covers.Publication{Name: old.Name, Digest: old.Digest, Skipped: true}, nil
		}
	}
	ext, _, err := covers.Inspect(data)
	if err != nil {
		return covers.Publication{}, err
	}
	name := "cover." + ext
	temp := ".onsei-cover-" + uuid.NewString() + ".tmp"
	f, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return covers.Publication{}, err
	}
	defer func() { _ = root.Remove(temp) }()
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return covers.Publication{}, err
	}
	journal, err := prepareCoverPublication(root, name, temp, data)
	if err != nil {
		return covers.Publication{}, err
	}

	if err = ctx.Err(); err != nil {
		return covers.Publication{}, err
	}
	j, err := root.OpenFile(coverJournal, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return covers.Publication{}, err
	}
	_, err = j.WriteString(covers.Encode(journal))
	if err == nil {
		err = j.Sync()
	}
	closeErr = j.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = root.Remove(coverJournal)
		return covers.Publication{}, err
	}
	if err = root.Rename(temp, name); err != nil {
		return covers.Publication{}, err
	}
	publication := covers.Publication{Name: name, Digest: journal.Digest}
	if err = recoverCover(root); err != nil {
		return publication, err
	}
	return publication, nil
}

func prepareCoverPublication(root *os.Root, name, temp string, data []byte) (coverPublication, error) {
	dir, err := root.Open(".")
	if err != nil {
		return coverPublication{}, err
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return coverPublication{}, err
	}
	journal := coverPublication{Name: name, Temp: temp, Digest: digest(data), Old: []string{}}
	for _, entry := range entries {
		if coverName(entry.Name()) {
			info, e := entry.Info()
			if e != nil {
				return coverPublication{}, e
			}
			if !info.Mode().IsRegular() {
				return coverPublication{}, covers.Failure(
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
