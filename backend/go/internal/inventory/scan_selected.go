package inventory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Clementine33/preen/backend/internal/librarypath"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

type selectedStaging interface {
	pipelineRepo
	MergeSelectedStaging(context.Context, string, string, []string) (MergeOutcome, error)
}

// ScanFoldersCtx shares one bounded queue, writer and selected-range merge.
// Folder walks may overlap; staging writes and the merge never do. The merge
// commits in several transactions, so a failed or canceled one may leave earlier
// folders merged; the listener is notified whenever a merge was attempted.
// Existing full and single-member scans keep their original pipeline and
// binding rules.
func (s *Pipeline) ScanFoldersCtx(
	ctx context.Context,
	folders []string,
	root string,
	workers int,
	opts ...ScanOption,
) (string, error) {
	scopes, err := SelectedScopes(root, folders)
	if err != nil || len(scopes) == 0 {
		return "", err
	}
	store, ok := s.repo.(selectedStaging)
	if !ok {
		return "", fmt.Errorf("selected staging capability unavailable")
	}
	var options scanOptions
	for _, option := range opts {
		option(&options)
	}
	id := uuid.NewString()
	root = librarypath.CleanRootPath(root)
	if err := s.repo.CreateScanSession(
		&ScanSession{SessionID: id, RootPath: root, Kind: "selected", Status: "running", StartedAt: time.Now()},
	); err != nil {
		return "", err
	}
	defer func() { _ = s.repo.CloseStagingSession(id) }()
	if err := s.stageSelected(ctx, store, id, root, scopes, workers, options.progress); err != nil {
		code, status := "SCAN_FAILED", "failed"
		if ctx.Err() != nil {
			code, status = "SCAN_CANCELLED", "canceled"
		}
		_ = s.repo.UpdateScanSessionStatus(id, status, code, err.Error())
		return "", err
	}
	if err := ctx.Err(); err != nil {
		_ = s.repo.UpdateScanSessionStatus(id, "canceled", "SCAN_CANCELLED", err.Error())
		return "", err
	}
	if err := s.repo.UpdateScanSessionStatus(id, "merging", "", ""); err != nil {
		return "", err
	}
	_, mergeErr := store.MergeSelectedStaging(ctx, id, root, scopes)
	s.notifyMerged()
	if mergeErr != nil {
		status, code := "failed", "MERGE_FAILED"
		if ctx.Err() != nil {
			status, code = "canceled", "SCAN_CANCELLED"
		}
		_ = s.repo.UpdateScanSessionStatus(id, status, code, mergeErr.Error())
		return "", mergeErr
	}
	if err := s.repo.UpdateScanSessionStatus(id, "completed", "", ""); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Pipeline) stageSelected(
	ctx context.Context,
	store selectedStaging,
	id, root string,
	scopes []string,
	workers int,
	progress func(Progress),
) error {
	if workers <= 0 {
		workers = DefaultScanWorkers
	}
	group, ctx := errgroup.WithContext(ctx)
	entries := make(chan StagingEntry, taskQueueSize)
	group.Go(func() error {
		batch := make([]StagingEntry, 0, pipelineBatchSize)
		tracker := newProgressTracker(progress)
		for entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			tracker.record(entry)
			batch = append(batch, entry)
			if len(batch) == pipelineBatchSize {
				if err := store.WriteStagingBatch(id, batch); err != nil {
					return err
				}
				batch = batch[:0]
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(batch) > 0 {
			if err := store.WriteStagingBatch(id, batch); err != nil {
				return err
			}
		}
		tracker.finish()
		return nil
	})
	var walkers sync.WaitGroup
	jobs := make(chan string)
	for range min(workers, len(scopes), 32) {
		walkers.Add(1)
		group.Go(func() error {
			defer walkers.Done()
			for folder := range jobs {
				if err := s.walkMember(ctx, folder, root, func(entry DirEntry) error {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case entries <- DirEntryToStagingEntry(entry, id, root):
						return nil
					}
				}); err != nil {
					return err
				}
			}
			return nil
		})
	}
	group.Go(func() error {
		defer close(jobs)
		for _, folder := range scopes {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case jobs <- folder:
			}
		}
		return nil
	})
	group.Go(func() error { walkers.Wait(); close(entries); return nil })
	return group.Wait()
}
