package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Clementine33/preen/backend/internal/admission"
)

func (s *Service) Start(ctx context.Context, categories []Category) (*Run, error) {
	categories, err := Normalize(categories)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil {
		return nil, context.Canceled
	}
	release, finish, err := s.gate.BeginCleanup()
	if err != nil {
		return nil, err
	}
	run := Run{
		ID:         uuid.NewString(),
		Categories: categories,
		Status:     "running",
		Phase:      categories[0],
		StartedAt:  time.Now().UTC(),
		Counts:     make(map[Category]Count),
	}
	for _, category := range categories {
		run.Counts[category] = Count{}
	}
	if err := s.store.CreateCleanupRun(ctx, run); err != nil {
		release()
		finish()
		return nil, err
	}
	workerCtx, cancel := context.WithCancel(s.ctx)
	s.cancel = cancel
	s.workers.Go(func() {
		defer func() { cancel(); s.mu.Lock(); s.cancel = nil; finish(); s.mu.Unlock() }()
		status, reason := s.execute(workerCtx, run, release)
		s.finalize(workerCtx, run.ID, status, reason)
	})
	return &run, nil
}

// Finalization outlives cancellation; shutdown's process guard owns the join deadline.
func (s *Service) finalize(ctx context.Context, id, status, reason string) {
	for {
		if ctx.Err() != nil {
			status, reason = s.failure(ctx)
		}
		finalCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		err := s.store.FinishCleanupRun(finalCtx, id, status, reason)
		stop()
		if err == nil {
			return
		}
		slog.Warn("cleanup finalization failed, retrying", "err", err)
		time.Sleep(250 * time.Millisecond)
	}
}

func (s *Service) execute(ctx context.Context, run Run, release func()) (string, string) {
	defer func() {
		if release != nil {
			release()
		}
	}()
	for _, category := range run.Categories {
		if err := s.store.SetCleanupPhase(ctx, run.ID, category); err != nil {
			return s.failure(ctx)
		}
		ids, err := s.store.CleanupCandidates(ctx, category, run.StartedAt, nil)
		if err != nil {
			return s.failure(ctx)
		}
		groups, err := s.store.CleanupGroups(ctx, category, ids)
		if err != nil {
			return s.failure(ctx)
		}
		for len(groups) > 0 {
			if ctx.Err() != nil {
				return s.failure(ctx)
			}
			batch := groups[0]
			groups = groups[1:]
			for len(groups) > 0 && len(batch)+len(groups[0]) <= s.opts.BatchRows {
				batch = append(batch, groups[0]...)
				groups = groups[1:]
			}
			if batchErr := s.store.CleanupBatch(
				ctx,
				run.ID,
				category,
				run.StartedAt,
				batch,
			); batchErr != nil {
				return s.failure(ctx)
			}
			release()
			release = nil
			timer := time.NewTimer(s.opts.YieldDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return s.failure(ctx)
			case <-timer.C:
			}
			release, err = s.gate.ContinueCleanup()
			if err != nil {
				if !admission.IsBusy(err) {
					return "failed", "Cleanup admission check failed; previously committed results are retained."
				}
				return "interrupted", "Cleanup yielded to other activity."
			}
		}
	}
	if ctx.Err() != nil {
		return s.failure(ctx)
	}
	return "succeeded", ""
}
func (s *Service) failure(ctx context.Context) (string, string) {
	if s.ctx.Err() != nil {
		return "interrupted", "Service stopped."
	}
	if ctx.Err() != nil {
		return "stopped", "Stop requested."
	}
	return "failed", "Cleanup batch failed; previously committed results are retained."
}

func (s *Service) Cancel(ctx context.Context, id string) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, err := s.store.StopCleanupRun(ctx, id)
	if err == nil && run.Status == "stopping" && s.cancel != nil {
		s.cancel()
	}
	return run, err
}

// Shutdown seals starts, cancels the process-owned worker and joins it before SQLite closes.
func (s *Service) Shutdown() {
	s.mu.Lock()
	s.closed = true
	s.stop()
	s.mu.Unlock()
	s.workers.Wait()
}
