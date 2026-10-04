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
	downloadCtx, cancel := context.WithCancel(ctx)
	active := &activeDownload{job: *job, cancel: cancel}
	s.activeMu.Lock()
	s.active[job.ID] = active
	s.activeMu.Unlock()
	defer func() {
		cancel()
		s.activeMu.Lock()
		if s.active[job.ID] == active {
			delete(s.active, job.ID)
		}
		s.activeMu.Unlock()
	}()
	done, err := s.process(downloadCtx, job)
	if ctx.Err() != nil {
		return true, ctx.Err()
	} // restart recovers the claimed row
	if downloadCtx.Err() != nil {
		err = nil
	}
	s.publication.Lock()
	defer s.publication.Unlock()
	if e := s.deps.Store.FinishCover(ctx, job.ID); e != nil {
		return true, errors.Join(err, e)
	}
	// Finish atomically requeues any remaining demands. Do not requeue again:
	// another worker may already have claimed that row.
	current, e := s.deps.Store.CoverCurrent(ctx, *job)
	if e != nil {
		return true, errors.Join(err, e)
	}
	if current {
		return done, err
	}
	if e := s.deps.Staging.Remove(job.ID, job.StagePath); e != nil {
		return true, errors.Join(err, e)
	}
	return true, err
}
func (s *Service) process(ctx context.Context, j *Job) (bool, error) {
	destinations, err := s.deps.Store.CoverDestinations(ctx, j.ID)
	if err != nil {
		return false, err
	}
	pending := false
	for i := range destinations {
		d := &destinations[i]
		if d.Status != "pending" {
			continue
		}
		skipped, e := s.skipExisting(ctx, d)
		if e != nil {
			return false, e
		}
		if skipped {
			continue
		}
		pending = true
	}
	if !pending {
		return true, nil
	}
	current, err := s.deps.Store.CoverCurrent(ctx, *j)
	if err != nil {
		return false, err
	}
	if !current {
		return true, nil
	}
	data, err := s.stage(ctx, j, destinations)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		for _, d := range destinations {
			if d.Status != "pending" {
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
	for _, d := range destinations {
		if d.Status != "pending" {
			continue
		}
		busy, e := s.deliver(ctx, *j, d, data)
		if busy {
			return false, nil
		}
		if e != nil {
			return false, e
		}
	}
	return true, nil
}

func (s *Service) skipExisting(ctx context.Context, d *Destination) (bool, error) {
	if d.Refresh || d.Published != "" {
		return false, nil
	}
	target, err := s.resolve(ctx, d.Target)
	if err != nil {
		return false, nil
	}
	image, err := s.deps.Files.Existing(target)
	if err != nil || image == nil || image.NeedsRecovery {
		return false, nil
	}
	d.Status = "skipped"
	d.Published = image.Name
	d.Digest = image.Digest
	return true, s.deps.Store.SaveCoverDestination(ctx, *d)
}
