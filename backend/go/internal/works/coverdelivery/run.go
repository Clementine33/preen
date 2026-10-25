package coverdelivery

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

// Run owns two bounded workers until shutdown. Recover must finish before startup.
func (s *Service) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			for ctx.Err() == nil {
				worked, err := s.ProcessNext(ctx)
				if err != nil && ctx.Err() == nil {
					log.Printf("covers: %v", err)
				}
				refreshed, refreshErr := s.RefreshNext(ctx)
				if refreshErr != nil && ctx.Err() == nil {
					log.Printf("metadata inventory refresh: %v", refreshErr)
				}
				worked = worked || refreshed
				if !worked || err != nil {
					timer := time.NewTimer(250 * time.Millisecond)
					select {
					case <-ctx.Done():
						timer.Stop()
					case <-timer.C:
					}
				}
			}
		})
	}
	wg.Wait()
	return ctx.Err()
}

// ProcessNext is one durable transfer/delivery pass, also used by integration tests.
func (s *Service) ProcessNext(ctx context.Context) (bool, error) {
	job, err := s.deps.Store.ClaimCover(ctx)
	if err != nil || job == nil {
		return false, err
	}
	ids := make([]string, len(job.Destinations))
	for i, d := range job.Destinations {
		ids[i] = d.ID
	}
	downloadCtx, cancel := context.WithCancel(ctx)
	active := &activeDownload{ids: ids, cancel: cancel}
	s.activeMu.Lock()
	s.active[ids[0]] = active
	s.activeMu.Unlock()
	defer func() {
		cancel()
		s.activeMu.Lock()
		delete(s.active, ids[0])
		s.activeMu.Unlock()
	}()
	done, err := s.process(downloadCtx, job, ids)
	if ctx.Err() != nil {
		return true, ctx.Err()
	} // restart recovers the claimed rows
	if downloadCtx.Err() != nil {
		err = nil
	}
	// Anything still running (admission busy, interrupted) goes back to pending.
	if e := s.deps.Store.FinishCover(ctx, ids); e != nil {
		return true, errors.Join(err, e)
	}
	return done, err
}
func (s *Service) process(ctx context.Context, j *Job, ids []string) (bool, error) {
	pending := false
	for i := range j.Destinations {
		skipped, e := s.skipExisting(ctx, &j.Destinations[i])
		if e != nil {
			return false, e
		}
		if !skipped {
			pending = true
		}
	}
	if !pending {
		return true, nil
	}
	current, err := s.deps.Store.CoverCurrent(ctx, ids)
	if err != nil {
		return false, err
	}
	if !current {
		return true, nil
	}
	data, digest, err := s.fetch(ctx, j)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		for _, d := range j.Destinations {
			if d.Status != "running" {
				continue
			}
			d.Status = "failed"
			d.ErrorCode = Code(err)
			d.ErrorMessage = err.Error()
			if e := s.deps.Store.SaveCoverDestination(ctx, d); e != nil {
				return false, e
			}
		}
		return true, nil
	}
	// Admission can stay busy for a long execution; keep the bytes and wait
	// instead of downloading again on every claim.
	remaining := []Destination{}
	for _, d := range j.Destinations {
		if d.Status == "running" {
			remaining = append(remaining, d)
		}
	}
	for len(remaining) > 0 {
		waiting := remaining[:0]
		for _, d := range remaining {
			busy, e := s.deliver(ctx, d, data, digest)
			if e != nil {
				return false, e
			}
			if busy {
				waiting = append(waiting, d)
			}
		}
		remaining = waiting
		if len(remaining) == 0 {
			break
		}
		ids := make([]string, len(remaining))
		for i, d := range remaining {
			ids[i] = d.ID
		}
		if current, e := s.deps.Store.CoverCurrent(ctx, ids); e != nil || !current {
			return e == nil, e
		}
		timer := time.NewTimer(s.deps.BusyRetry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, ctx.Err()
		case <-timer.C:
		}
	}
	return true, nil
}

func (s *Service) skipExisting(ctx context.Context, d *Destination) (bool, error) {
	if d.Refresh || d.Published != "" {
		return false, nil
	}
	image, err := s.deps.Files.Existing(d.Target)
	if err != nil || image == nil || image.NeedsRecovery {
		return false, nil
	}
	d.Status = "skipped"
	d.Published = image.Name
	d.Digest = image.Digest
	if s.deps.SyncInventory != nil {
		if err := s.deps.SyncInventory(ctx, d.Target, Publication{
			Name: image.Name, Digest: image.Digest, Skipped: true, File: &image.File,
		}); err != nil {
			d.Status, d.ErrorCode, d.ErrorMessage = "failed", "INVENTORY_REFRESH_FAILED", err.Error()
		}
	}
	return true, s.deps.Store.SaveCoverDestination(ctx, *d)
}
