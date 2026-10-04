package coverdelivery

import (
	"context"

	"github.com/onsei/organizer/backend/internal/admission"
)

func (s *Service) deliver(ctx context.Context, j Job, d Destination, data []byte) (bool, error) {
	release, err := s.deps.Acquire()
	if admission.IsBusy(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer release()
	s.publication.Lock()
	defer s.publication.Unlock()
	current, err := s.deps.Store.CoverCurrent(ctx, j)
	if err != nil {
		return false, err
	}
	if !current {
		return false, nil
	}
	all, err := s.deps.Store.CoverDestinations(ctx, j.ID)
	if err != nil {
		return false, err
	}
	active := false
	for _, item := range all {
		if item.ID == d.ID && item.Status == "pending" {
			active = true
		}
	}
	if !active {
		return false, nil
	}
	target := d.Target
	result, err := s.deps.Files.Publish(ctx, target, data, j.Digest, d.Refresh)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		d.Status = "failed"
		d.ErrorCode = Code(err)
		d.ErrorMessage = err.Error()
		d.Published = result.Name
		d.Digest = result.Digest
	} else {
		d.Status = "succeeded"
		if result.Skipped {
			d.Status = "skipped"
		}
		d.Published = result.Name
		d.Digest = result.Digest
		if !result.Skipped {
			err = s.deps.Store.SaveStoredCover(
				ctx,
				target.CopyID,
				Stored{
					URL:          j.URL,
					Name:         result.Name,
					Digest:       result.Digest,
					ETag:         j.ETag,
					LastModified: j.LastModified,
				},
			)
		}
	}
	// Persist actual changes even when old-extension cleanup partially failed.
	if (result.File != nil || len(result.Removed) > 0) && s.deps.SyncInventory != nil {
		if e := s.deps.SyncInventory(ctx, target, result); e != nil {
			d.Status = "failed"
			d.ErrorCode = "INVENTORY_REFRESH_FAILED"
			d.ErrorMessage = e.Error()
		}
	}
	if err == nil && d.ErrorCode == "" {
		if e := s.deps.Files.CompletePublication(target); e != nil {
			d.Status, d.ErrorCode, d.ErrorMessage = "failed", Code(e), e.Error()
		}
	}
	if err != nil && d.ErrorCode == "" {
		return false, err
	}
	return false, s.deps.Store.SaveCoverDestination(ctx, d)
}
