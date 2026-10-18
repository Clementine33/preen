package workset_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/workset"
)

type shutdownGenerationTask struct {
	stubTask

	entered chan struct{}
	failure error
}

func (s shutdownGenerationTask) PlanSession(
	ctx context.Context,
	_ workset.PlanSessionInput,
) (*workset.PlanSnapshot, error) {
	close(s.entered)
	<-ctx.Done()
	if s.failure != nil {
		return nil, s.failure
	}
	return nil, ctx.Err()
}

func TestGenerationShutdownInterruptsAndReleasesKey(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "interrupted", true: "real_failure"}[fail], func(t *testing.T) {
			f := newFixture(t)
			task := shutdownGenerationTask{entered: make(chan struct{}),
				kind: "shutdown", seed: `{}`}
			if fail {
				task.failure = errors.New("independent planning failure")
			}
			workset.RegisterTasksForTest(f.svc, []workset.Task{task})
			ids := f.standardLibrary("albumA")
			ws := f.createCurrentOperation("lib-1", task.kind, ids...)
			op := f.operationFor(ws.WorksetID, task.kind)
			started, err := f.svc.StartGeneration(
				t.Context(),
				ws.WorksetID,
				task.kind,
				workset.StartGenerationRequest{IfMatchVersion: op.Version, IdempotencyKey: "shutdown-key"},
			)
			if err != nil {
				t.Fatal(err)
			}
			f.svc.DispatcherHandle().Start()
			select {
			case <-task.entered:
			case <-time.After(4 * time.Second):
				t.Fatal("generation did not start")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
			defer cancel()
			if stopErr := f.svc.DispatcherHandle().Stop(ctx); stopErr != nil {
				t.Fatal(stopErr)
			}
			g, err := f.repo.GetGeneration(started.Generation.GenerationID)
			if err != nil {
				t.Fatal(err)
			}
			want := workset.GenStatusInterrupted
			if fail {
				want = workset.GenStatusFailed
			}
			if g.Status != want || g.ErrorCode == "CANCELED" || g.CancelRequested {
				t.Fatalf("generation = %+v", g)
			}
			replay, err := f.svc.StartGeneration(t.Context(), ws.WorksetID, task.kind, workset.StartGenerationRequest{
				IfMatchVersion: op.Version, IdempotencyKey: "shutdown-key",
			})
			if err != nil || !replay.Created || replay.Generation.GenerationID == g.GenerationID {
				t.Fatalf("generation key not released: %+v %v", replay, err)
			}
		})
	}
}

func TestExecutionShutdownJoinsEncodersAndPreservesPublishedFacts(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "encode", true: "commit"}[commit], func(t *testing.T) {
			plan := poolUnit(1)
			if commit {
				plan.Tasks, plan.CommitGate = 0, true
			}
			f, obs := poolFixture(t, 1, plan)
			if !commit {
				obs.gate("u0")
			}
			f.seedRevision("plan-stop", poolComponent("u0", "albumA"))
			started := f.mustStart("plan-stop", "stop-key")
			if commit {
				obs.waitEvent(t, "commit u0")
			} else {
				obs.waitEncode(t, "u0")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
			defer cancel()
			if stopErr := f.svc.DispatcherHandle().Stop(ctx); stopErr != nil {
				t.Fatal(stopErr)
			}
			done := f.waitTerminal(started.ExecutionID)
			if done.Status != workset.ExecStatusInterrupted || done.ErrorCode != "INTERRUPTED" {
				t.Fatalf("execution = %+v", done)
			}
			obs.mu.Lock()
			running, open := obs.running, obs.open
			obs.mu.Unlock()
			if running != 0 || open != 0 {
				t.Fatalf("running=%d open=%d", running, open)
			}
			if commit && len(entryOf(t, done, "u0").Committed) != 1 {
				t.Fatal("published facts lost")
			}
		})
	}
}

func TestExecutionFailureDuringShutdownKeepsFailure(t *testing.T) {
	plan := poolUnit(1)
	plan.FinishOnStop, plan.FailEncode = true, 0
	f, obs := poolFixture(t, 1, plan)
	obs.gate("u0")
	defer obs.release("u0")
	f.seedRevision("plan-failed-stop", poolComponent("u0", "albumA"))
	started := f.mustStart("plan-failed-stop", "failed-stop-key")
	obs.waitEncode(t, "u0")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	err := f.svc.DispatcherHandle().Stop(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), started.ExecutionID) {
		t.Fatalf("Stop = %v", err)
	}
	obs.release("u0")
	ctx, cancel = context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	if err := f.svc.DispatcherHandle().Stop(ctx); err != nil {
		t.Fatal(err)
	}
	done := f.waitTerminal(started.ExecutionID)
	if done.Status != workset.ExecStatusFailed || done.ErrorCode != "ENCODE_FAILED" {
		t.Fatalf("execution = %+v", done)
	}
}
