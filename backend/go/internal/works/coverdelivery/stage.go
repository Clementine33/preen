package coverdelivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func (s *Service) stage(ctx context.Context, j *Job, targets []Destination) ([]byte, error) {
	path := filepath.Join(s.deps.StageDir, j.ID+".image")
	if j.StagePath == path {
		data, err := readStage(path)
		if err == nil {
			ext, _, e := Inspect(data)
			sum := sha256.Sum256(data)
			if e == nil && hex.EncodeToString(sum[:]) == j.Digest && ext == j.Extension {
				return data, nil
			}
		}
	}
	if j.URL == "" && !j.MainChecked && s.deps.Source != nil {
		j.URL, _ = s.deps.Source.FetchMain(ctx, j.WorkCode, "dlsite", j.Locale)
		j.MainChecked = true
	}
	result, err := s.download(ctx, j, targets)
	if err != nil && ctx.Err() == nil {
		backup := j.BackupURL
		if backup == "" && !j.BackupChecked && s.deps.Source != nil {
			backup, _ = s.deps.Source.FetchMain(ctx, j.WorkCode, "asmr_one", j.Locale)
			j.BackupChecked = true
		}
		if backup != "" {
			j.URL = backup
			result, err = s.deps.Downloader.Download(ctx, j.URL, Validators{})
		}
	}
	if err != nil {
		return nil, err
	}
	ext, _, err := Inspect(result.Data)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(result.Data)
	j.Digest = hex.EncodeToString(sum[:])
	j.Extension = ext
	j.ETag = result.ETag
	j.LastModified = result.LastModified
	temp := path + ".tmp"
	defer func() { _ = os.Remove(temp) }()
	if err = os.WriteFile(temp, result.Data, 0600); err != nil {
		return nil, err
	}
	if err = os.Rename(temp, path); err != nil {
		return nil, err
	}
	j.StagePath = path
	if err = s.deps.Store.SaveCoverStage(ctx, *j); err != nil {
		return nil, err
	}
	return result.Data, nil
}
func (s *Service) download(ctx context.Context, j *Job, targets []Destination) (Download, error) {
	v := Validators{}
	var local *Existing
	if j.Refresh {
		for _, d := range targets {
			target, e := s.resolve(ctx, d.Target)
			if e != nil {
				continue
			}
			saved, e := s.deps.Store.StoredCover(ctx, target.CopyID)
			if e != nil {
				return Download{}, e
			}
			if saved == nil || saved.URL != j.URL {
				continue
			}
			image, e := s.deps.Files.Existing(target)
			if e == nil && image != nil && image.Digest == saved.Digest && image.Name == saved.Name {
				local = image
				v = Validators{ETag: saved.ETag, LastModified: saved.LastModified}
				break
			}
		}
	}
	if j.URL == "" {
		return Download{}, fmt.Errorf("no main cover URL")
	}
	result, err := s.deps.Downloader.Download(ctx, j.URL, v)
	if err == nil && result.NotModified {
		if local == nil {
			return result, fmt.Errorf("304 without a matching local file")
		}
		result.Data = local.Data
		result.Extension = local.Extension
		if result.ETag == "" {
			result.ETag = v.ETag
		}
		if result.LastModified == "" {
			result.LastModified = v.LastModified
		}
	}
	return result, err
}

func readStage(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, MaxBytes+1))
}
