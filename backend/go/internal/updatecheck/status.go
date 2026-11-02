package updatecheck

import (
	"context"
	"time"
)

// Status returns the current result.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Service) settle(status Status) {
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
