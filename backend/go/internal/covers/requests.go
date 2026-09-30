package covers

import (
	"context"
	"fmt"
	"path/filepath"
)

func (s *Service) Enqueue(ctx context.Context, r Request) error {
	if r.ExecutionID == "" || r.WorkCode == "" {
		return fmt.Errorf("missing cover ownership")
	}
	s.publication.Lock()
	err := s.deps.Store.EnqueueCovers(ctx, r)
	s.publication.Unlock()
	if err == nil {
		err = s.stopUnneeded(ctx)
	}
	return err
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
func (s *Service) resolve(ctx context.Context, t Target) (Target, error) {
	now, err := s.deps.Store.ResolveCoverCopy(ctx, t.CopyID)
	if err != nil {
		return t, err
	}
	if now.WorkCode != t.WorkCode || filepath.Clean(now.LibraryRoot) != filepath.Clean(t.LibraryRoot) ||
		now.RelPath != t.RelPath {
		return t, changed()
	}
	now.Identity = t.Identity
	return now, nil
}

func (s *Service) stopUnneeded(ctx context.Context) error {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	for _, active := range s.active {
		current, err := s.deps.Store.CoverCurrent(ctx, active.job)
		if err != nil {
			return err
		}
		if !current {
			active.cancel()
		}
	}
	return nil
}
