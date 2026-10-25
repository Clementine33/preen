package coverdelivery

import (
	"context"

	"github.com/Clementine33/preen/backend/internal/admission"
)

func (s *Service) deliver(ctx context.Context, d Destination, data []byte, digest string) (bool, error) {
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
	current, err := s.deps.Store.CoverCurrent(ctx, []string{d.ID})
	if err != nil || !current || ctx.Err() != nil {
		return false, err
	}
	target := d.Target
	result, err := s.deps.Files.Publish(ctx, target, data, digest, d.Refresh)
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
