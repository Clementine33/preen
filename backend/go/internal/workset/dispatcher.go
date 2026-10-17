package workset

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
)

var errShutdown = errors.New("dispatcher shutdown")

// dispatcher owns generation workers and one execution worker, including every
// session and child goroutine they start. Database queue order is independent of
// process shutdown: a claim returning after stop is recovered at next startup.
type dispatcher struct {
	svc        *serviceImpl
	workers    int
	wakeC      chan struct{}
	lightWakeC chan struct{}
	execWakeC  chan struct{}
	done       chan struct{}
	joined     chan struct{}
	ctx        context.Context
	cancel     context.CancelCauseFunc
	wg         sync.WaitGroup
	mu         sync.Mutex
	started    bool
	stopped    bool
	loops      int
	active     map[string]struct{}
}

func newDispatcher(svc *serviceImpl, workers int) *dispatcher {
	if workers <= 0 {
		workers = 2
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	return &dispatcher{
		svc: svc, workers: workers, wakeC: make(chan struct{}, 1), execWakeC: make(chan struct{}, 1),
		lightWakeC: make(chan struct{}, 1),
		done:       make(chan struct{}), joined: make(chan struct{}), ctx: ctx, cancel: cancel,
		active: make(map[string]struct{}),
	}
}

// Start registers all workers before Stop can start waiting.
func (d *dispatcher) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started || d.stopped {
		return
	}
	d.started = true
	d.loops = d.workers + 2
	for range d.workers {
		d.wg.Go(func() { defer d.loopExited(); d.run(nil, d.wakeC) })
	}
	lightKinds := make([]string, 0)
	for _, task := range d.svc.tasks {
		if _, ok := task.(LightPlanner); ok {
			lightKinds = append(lightKinds, task.Kind())
		}
	}
	d.wg.Go(func() { defer d.loopExited(); d.run(lightKinds, d.lightWakeC) })
	d.wg.Go(func() { defer d.loopExited(); d.runExecutionLoop() })
	go func() { d.wg.Wait(); close(d.joined) }()
}

// Stop seals admission and interrupts sessions, then joins their workers.
// A timeout leaves the database open; the application owns the process deadline.
func (d *dispatcher) Stop(ctx context.Context) error {
	d.mu.Lock()
	if !d.stopped {
		d.stopped = true
		close(d.done)
		d.cancel(errShutdown)
		if !d.started {
			close(d.joined)
		}
	}
	d.mu.Unlock()
	select {
	case <-d.joined:
		return nil
	default:
	}
	select {
	case <-d.joined:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("dispatcher stop: %w (%s)", ctx.Err(), d.pending())
	}
}

func (d *dispatcher) loopExited() {
	d.mu.Lock()
	d.loops--
	d.mu.Unlock()
}

func (d *dispatcher) beginSession(kind, id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return false
	}
	d.active[kind+" "+id] = struct{}{}
	return true
}

func (d *dispatcher) endSession(kind, id string) {
	d.mu.Lock()
	delete(d.active, kind+" "+id)
	d.mu.Unlock()
}

func (d *dispatcher) pending() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	active := make([]string, 0, len(d.active))
	for name := range d.active {
		active = append(active, name)
	}
	slices.Sort(active)
	return fmt.Sprintf("workers=%d sessions=%v", d.loops, active)
}

func cancellationOutcome(ctx context.Context) (status, code, message string) {
	if errors.Is(context.Cause(ctx), errShutdown) {
		return ExecStatusInterrupted, "INTERRUPTED", "execution interrupted by shutdown"
	}
	return ExecStatusCanceled, "CANCELED", "execution canceled"
}

func sessionCanceled(ctx context.Context, err error) bool {
	if isCanceled(err) {
		return true
	}
	if ctx.Err() != nil && errors.Is(err, context.Cause(ctx)) {
		return true
	}
	e, ok := AsError(err)
	return ok && e.Code == "CANCELED"
}

func (d *dispatcher) finishGenerationCancellation(ctx context.Context, id string) {
	var err error
	if errors.Is(context.Cause(ctx), errShutdown) {
		err = d.svc.repo.MarkGenerationInterrupted(id)
	} else {
		err = d.svc.repo.CompleteGenerationCanceled(id)
	}
	if err != nil {
		logGenerationTerminalFailure(id, err)
	}
}

// String reports live workers and sessions for the shutdown watchdog.
func (d *dispatcher) String() string { return d.pending() }
