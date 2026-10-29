package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
)

const retiredPlanBatchRows = 1000

// retiredPlans owns the single process worker for invisible plan data. It
// resumes from persisted retirement markers; no in-memory cursor survives a
// batch or needs recovery. It never acquires file-management admission.
type retiredPlans struct {
	repo *sqlite.Repository
	wake chan struct{}
}

func startRetiredPlans(ctx context.Context, repo *sqlite.Repository) (*retiredPlans, <-chan struct{}) {
	c := &retiredPlans{repo: repo, wake: make(chan struct{}, 1)}
	repo.OnPlansRetired = c.kick
	c.drain(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.run(ctx)
	}()
	return c, done
}

func (c *retiredPlans) kick() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// drain is also called synchronously before opening dispatcher admission.
func (c *retiredPlans) drain(ctx context.Context) {
	for ctx.Err() == nil {
		rows, err := c.repo.DeleteRetiredPlanBatch(ctx, retiredPlanBatchRows)
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("retired plan cleanup failed", "err", err)
			}
			return
		}
		if rows == 0 || !wait(ctx, time.Millisecond) {
			return
		}
	}
}

func (c *retiredPlans) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.wake:
			c.drain(ctx)
		}
	}
}
