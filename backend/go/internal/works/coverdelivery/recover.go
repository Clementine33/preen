package coverdelivery

import (
	"context"
)

// Recover is called before workers start; stopped requests remain stopped.
func (s *Service) Recover(ctx context.Context) error {
	if err := s.deps.Store.RecoverCovers(ctx); err != nil {
		return err
	}
	active, err := s.deps.Store.ActiveCoverJobs(ctx)
	if err != nil {
		return err
	}
	return s.deps.Staging.Cleanup(active)
}
