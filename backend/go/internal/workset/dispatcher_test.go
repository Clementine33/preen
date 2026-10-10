//nolint:testpackage // Late-claim and lifecycle races need direct access to dispatcher admission.
package workset

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

type delayedClaimStore struct {
	Store

	entered chan struct{}
	release chan struct{}
}

func (s *delayedClaimStore) NextQueuedGeneration() (*PlanGeneration, error) {
	close(s.entered)
	<-s.release
	return &PlanGeneration{GenerationID: "late", Status: GenStatusRunning}, nil
}
func (*delayedClaimStore) NextQueuedExecution() (*PlanExecution, error) { return nil, nil }

func TestDispatcherStopJoinsLateClaimWithoutStartingIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &delayedClaimStore{entered: make(chan struct{}), release: make(chan struct{})}
		d := newDispatcher(&serviceImpl{repo: store}, 1)
		d.Start()
		<-store.entered
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		stopped := make(chan error, 1)
		go func() { stopped <- d.Stop(ctx) }()
		<-d.ctx.Done()
		synctest.Wait()
		select {
		case err := <-stopped:
			t.Fatalf("Stop did not join claim: %v", err)
		default:
		}
		close(store.release)
		if err := <-stopped; err != nil {
			t.Fatal(err)
		}
		if !errors.Is(context.Cause(d.ctx), errShutdown) {
			t.Fatal("shutdown cause lost")
		}
		if err := d.Stop(ctx); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDispatcherStopBeforeStart(t *testing.T) {
	d := newDispatcher(nil, 1)
	if err := d.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	d.Start()
	if err := d.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestCancellationClassificationKeepsRealFailure(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(errShutdown)
	if sessionCanceled(ctx, errors.New("disk failure")) {
		t.Fatal("shutdown masked a real failure")
	}
	if !sessionCanceled(ctx, context.Canceled) {
		t.Fatal("cooperative stop not recognized")
	}
	status, code, _ := cancellationOutcome(ctx)
	if status != ExecStatusInterrupted || code != "INTERRUPTED" {
		t.Fatalf("%s %s", status, code)
	}
}
