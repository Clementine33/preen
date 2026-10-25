package coverdelivery

import (
	"context"
)

// Recover is called before workers start; stopped requests remain stopped.
func (s *Service) Recover(ctx context.Context) error {
	return s.deps.Store.RecoverCovers(ctx)
}
