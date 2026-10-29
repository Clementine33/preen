package coverdelivery

import (
	"context"
	"fmt"
	"log/slog"
)

// WithPublication runs a storage write before or after a whole publication,
// never between its demand recheck and file publish. The write must open and
// finish its own transaction here, after the publication lock is acquired.
func (s *Service) WithPublication(ctx context.Context, write func() error) error {
	s.publication.Lock()
	err := write()
	s.publication.Unlock()
	if err == nil {
		if stopErr := s.stopUnneeded(ctx); stopErr != nil {
			// The write has committed. deliver still rechecks demand before publishing.
			slog.Error("stop retired cover downloads failed", "err", stopErr)
		}
	}
	return err
}

func (s *Service) Enqueue(ctx context.Context, r Request) error {
	if r.ExecutionID == "" || r.WorkCode == "" {
		return fmt.Errorf("missing cover ownership")
	}
	return s.deps.Store.EnqueueCovers(ctx, r)
}
func (s *Service) Report(ctx context.Context, execution string) (Report, error) {
	return s.deps.Store.CoverReport(ctx, execution)
}
func (s *Service) Retry(ctx context.Context, execution string) error {
	return s.deps.Store.ChangeCoverRequests(ctx, execution, true)
}
func (s *Service) Cancel(ctx context.Context, execution string) error {
	s.publication.Lock()
	err := s.deps.Store.ChangeCoverRequests(ctx, execution, false)
	s.publication.Unlock()
	if err == nil {
		err = s.stopUnneeded(ctx)
	}
	return err
}
func (s *Service) Read(ctx context.Context, code, copyID string) (*Existing, error) {
	target, err := s.deps.Store.ResolveCoverCopy(ctx, copyID)
	if err != nil {
		return nil, err
	}
	if target.WorkCode != code {
		return nil, unavailable("copy does not belong to work")
	}
	image, err := s.deps.Files.Existing(target)
	if err != nil {
		return nil, err
	}
	if image == nil {
		return nil, unavailable("no local cover")
	}
	return image, nil
}
func (s *Service) stopUnneeded(ctx context.Context) error {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	for _, active := range s.active {
		current, err := s.deps.Store.CoverCurrent(ctx, active.ids)
		if err != nil {
			return err
		}
		if !current {
			active.cancel()
		}
	}
	return nil
}
