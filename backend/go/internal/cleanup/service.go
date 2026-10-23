package cleanup

import (
	"context"
	"sync"
	"time"
)

type Service struct {
	store   Store
	gate    Gate
	opts    Options
	ctx     context.Context
	stop    context.CancelFunc
	mu      sync.Mutex
	cancel  context.CancelFunc
	closed  bool
	workers sync.WaitGroup
}

func NewService(ctx context.Context, store Store, gate Gate, opts Options) *Service {
	ctx, stop := context.WithCancel(ctx)
	if opts.BatchRows <= 0 {
		opts.BatchRows = 500
	}
	if opts.YieldDelay <= 0 {
		opts.YieldDelay = 250 * time.Millisecond
	}
	return &Service{store: store, gate: gate, opts: opts, ctx: ctx, stop: stop}
}
