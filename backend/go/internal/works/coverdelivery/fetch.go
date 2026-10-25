package coverdelivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// fetch downloads the work's current cover once. When that URL is unknown,
// rejected or fails, the ASMR.one backup is tried instead.
func (s *Service) fetch(ctx context.Context, j *Job) ([]byte, string, error) {
	url, err := s.deps.Store.WorkCoverURL(ctx, j.WorkCode)
	if err != nil {
		return nil, "", err
	}
	result, err := Download{}, fmt.Errorf("no main cover URL")
	if url != "" {
		result, err = s.deps.Downloader.Download(ctx, url)
	}
	if err != nil && ctx.Err() == nil && s.deps.Source != nil {
		backup, _ := s.deps.Source.FetchMain(ctx, j.WorkCode, "asmr_one", j.Locale)
		if backup != "" && backup != url {
			result, err = s.deps.Downloader.Download(ctx, backup)
		}
	}
	if err != nil {
		return nil, "", err
	}
	if _, _, err = Inspect(result.Data); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(result.Data)
	return result.Data, hex.EncodeToString(sum[:]), nil
}
