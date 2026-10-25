package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

// Publish reports only the files this publication changed. The journal remains
// until the caller has persisted those facts, so inventory failures are retryable.
func (CoverFiles) Publish(
	ctx context.Context, t coverdelivery.Target, data []byte, sum string, refresh bool,
) (coverdelivery.Publication, error) {
	root, err := openCoverRoot(t)
	if err != nil {
		return coverdelivery.Publication{}, err
	}
	defer root.Close()
	prior, err := recoverCover(root, t)
	if err != nil || prior.Name != "" && prior.Digest == sum {
		return prior, err
	}
	// Refresh replaces a differing or misnamed cover but leaves identical bytes untouched.
	old, e := existingCover(root)
	if e != nil && !refresh {
		return prior, e
	}
	if e == nil && old != nil && (!refresh || old.Digest == sum && old.Name == "cover."+old.Extension) {
		old.File.Path = coverPath(t, old.Name)
		return coverdelivery.Publication{
			Name: old.Name, Digest: old.Digest, Skipped: true, File: &old.File, Removed: prior.Removed,
		}, nil
	}
	return publishCover(ctx, root, t, data, sum, prior)
}

func publishCover(
	ctx context.Context,
	root *os.Root,
	t coverdelivery.Target,
	data []byte,
	sum string,
	prior coverdelivery.Publication,
) (coverdelivery.Publication, error) {
	ext, _, err := coverdelivery.Inspect(data)
	if err != nil {
		return prior, err
	}
	name, temp := "cover."+ext, ".onsei-cover-"+uuid.NewString()+".tmp"
	facts, err := writeCoverFile(root, temp, data)
	defer func() { _ = root.Remove(temp) }()
	if err != nil {
		return prior, err
	}
	journal, err := prepareCoverPublication(root, name, temp, sum)
	if err != nil {
		return prior, err
	}
	// Carry a prior publication's deletions forward if a newer candidate arrives
	// before its inventory acknowledgment.
	for _, removed := range prior.Removed {
		journal.Old = append(journal.Old, filepath.Base(removed))
	}
	if err = ctx.Err(); err != nil {
		return prior, err
	}
	if err = writeCoverJournal(root, journal); err != nil {
		return prior, err
	}
	if err = root.Rename(temp, name); err != nil {
		return prior, err
	}
	facts.Path = coverPath(t, name)
	result := coverdelivery.Publication{Name: name, Digest: sum, File: &facts}
	return cleanCoverPublication(root, t, journal, result)
}

// writeCoverFile captures inventory facts from the writer, without reading the
// newly published bytes or comparing timestamps after close.
func writeCoverFile(root *os.Root, name string, data []byte) (inventory.InventoryFile, error) {
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return inventory.InventoryFile{}, err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	var facts inventory.InventoryFile
	if err == nil {
		var info os.FileInfo
		info, err = f.Stat()
		if err == nil {
			facts.Size, facts.Mtime = info.Size(), info.ModTime().Unix()
		}
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return facts, err
}

func writeCoverJournal(root *os.Root, journal coverPublication) error {
	// A newer candidate carries the previous delta forward. Replace its journal
	// only after the new one is complete, preserving retry facts on write failure.
	temp := ".onsei-cover-journal-" + uuid.NewString() + ".tmp"
	defer func() { _ = root.Remove(temp) }()
	j, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = j.WriteString(coverdelivery.Encode(journal)); err == nil {
		err = j.Sync()
	}
	if closeErr := j.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = root.Rename(temp, coverJournal)
	}
	return err
}

// recoverCover reads the image only when a previous publication journal exists.
// An unverified landing never authorizes removing old cover files.
func recoverCover(root *os.Root, t coverdelivery.Target) (coverdelivery.Publication, error) {
	result := coverdelivery.Publication{}
	info, err := root.Lstat(coverJournal)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() || info.Size() > 8192 {
		return result, coverdelivery.Failure("COVER_CONFLICT", fmt.Errorf("invalid publication journal"))
	}
	raw, err := root.ReadFile(coverJournal)
	if err != nil {
		return result, err
	}
	var j coverPublication
	if err = json.Unmarshal(raw, &j); err != nil {
		return result, err
	}
	if !coverName(j.Name) || !strings.HasPrefix(j.Temp, ".onsei-cover-") || !strings.HasSuffix(j.Temp, ".tmp") ||
		filepath.Base(j.Temp) != j.Temp {
		return result, fmt.Errorf("invalid cover journal paths")
	}
	image, readErr := readImage(root, j.Name)
	if readErr == nil && image.Digest == j.Digest {
		image.File.Path = coverPath(t, image.Name)
		result = coverdelivery.Publication{Name: image.Name, Digest: image.Digest, File: &image.File}
		return cleanCoverPublication(root, t, j, result)
	}
	// A newer candidate can replace an unacknowledged journal before landing.
	// Preserve already absent old paths for inventory sync; a digest mismatch
	// never authorizes removing surviving covers.
	for _, old := range j.Old {
		if !coverName(old) || old == j.Name {
			continue
		}
		_, statErr := root.Lstat(old)
		if os.IsNotExist(statErr) {
			result.Removed = append(result.Removed, coverPath(t, old))
		} else if statErr != nil {
			return result, statErr
		}
	}
	if err = root.Remove(j.Temp); err != nil && !os.IsNotExist(err) {
		return result, err
	}
	// Keep the deletion evidence until inventory acknowledgment or a complete
	// replacement journal, even when the interrupted candidate did not land.
	return result, nil
}

func cleanCoverPublication(
	root *os.Root, t coverdelivery.Target, j coverPublication, result coverdelivery.Publication,
) (coverdelivery.Publication, error) {
	for _, old := range j.Old {
		if !coverName(old) || old == j.Name {
			continue
		}
		info, err := root.Lstat(old)
		if !os.IsNotExist(err) {
			if err != nil {
				return result, err
			}
			if !info.Mode().IsRegular() {
				return result, coverdelivery.Failure(
					"COVER_CONFLICT",
					fmt.Errorf("old cover is no longer a regular file"),
				)
			}
			if err = root.Remove(old); err != nil && !os.IsNotExist(err) {
				return result, err
			}
		}
		// Already absent names may have been removed before a failed inventory
		// sync. The journal carries their exact paths into the retry's delta.
		result.Removed = append(result.Removed, coverPath(t, old))
	}
	if err := root.Remove(j.Temp); err != nil && !os.IsNotExist(err) {
		return result, err
	}
	return result, nil
}

// CompletePublication drops only the journal; it never rereads or validates the image.
func (CoverFiles) CompletePublication(t coverdelivery.Target) error {
	root, err := os.OpenRoot(t.LibraryRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	if err = root.Remove(filepath.Join(t.RelPath, coverJournal)); os.IsNotExist(err) {
		return nil
	}
	return err
}

func coverPath(t coverdelivery.Target, name string) string {
	return filepath.ToSlash(filepath.Join(t.LibraryRoot, t.RelPath, name))
}
