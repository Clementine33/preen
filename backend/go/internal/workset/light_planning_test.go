package workset_test

import (
	"context"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/workset"
)

type gatedPlanner struct {
	stubTask

	entered chan struct{}
	release chan struct{}
	slot    chan struct{}
}

func (s gatedPlanner) RefreshGenerationInputs(
	ctx context.Context, _ []string, _ string, _ func(), _ func() error,
) error {
	s.entered <- struct{}{}
	if s.slot != nil {
		select {
		case s.slot <- struct{}{}:
			defer func() { <-s.slot }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func newGatedPlanner(kind string) gatedPlanner {
	var planner gatedPlanner
	planner.kind, planner.seed = kind, `{}`
	planner.entered = make(chan struct{}, 1)
	return planner
}

type lightTestPlanner struct{ gatedPlanner }

func (lightTestPlanner) LightPlanning() {}

func enqueuePlanner(t *testing.T, f *fixture, id, kind string) string {
	t.Helper()
	op := f.operationFor(id, kind)
	result, err := f.svc.StartGeneration(t.Context(), id, kind, workset.StartGenerationRequest{
		IfMatchVersion: op.Version, IdempotencyKey: "light-lane-" + kind,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result.Generation.GenerationID
}

func waitPlanner(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(4 * time.Second):
		t.Fatal("planner did not enter")
	}
}

func waitPlannerCompleted(t *testing.T, f *fixture, id string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for {
		g, getErr := f.repo.GetGeneration(id)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if g.Status == workset.GenStatusCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("light planner did not complete: %+v", g)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLightPlanningBypassesFullGeneralPoolAndStops(t *testing.T) {
	f := newFixture(t)
	slot := make(chan struct{}, 1)
	heavy, waiting := newGatedPlanner("heavy"), newGatedPlanner("waiting")
	heavy.slot, waiting.slot = slot, slot
	light := lightTestPlanner{newGatedPlanner("light")}
	light.release = make(chan struct{}, 1)
	undeclared := stubTask{kind: "undeclared", seed: `{}`}
	f.svc = workset.NewService(f.repo, 2, 1, []workset.Task{heavy, waiting, light, undeclared}, nil, nil)
	ids := f.standardLibrary("albumA")
	ws := f.createCurrentOperation("lib-1", heavy.kind, ids...)
	heavyID := enqueuePlanner(t, f, ws.WorksetID, heavy.kind)
	waitingID := enqueuePlanner(t, f, ws.WorksetID, waiting.kind)
	handle := f.svc.DispatcherHandle()
	handle.Start()
	t.Cleanup(func() { _ = handle.Stop(context.Background()) })
	waitPlanner(t, heavy.entered)
	waitPlanner(t, waiting.entered)
	undeclaredID := enqueuePlanner(t, f, ws.WorksetID, undeclared.kind)
	lightID := enqueuePlanner(t, f, ws.WorksetID, light.kind)
	waitPlanner(t, light.entered)
	queued, err := f.repo.GetGeneration(undeclaredID)
	if err != nil || queued.Status != workset.GenStatusQueued {
		t.Fatalf("undeclared task entered light lane: %+v %v", queued, err)
	}
	light.release <- struct{}{}
	waitPlannerCompleted(t, f, lightID)
	// A new light session stays active until Stop, alongside both general workers.
	if _, err = f.repo.DB().
		Exec("UPDATE workset_operations SET current_revision_id = NULL WHERE operation_type = ?", light.kind); err != nil {
		t.Fatal(err)
	}
	result, err := f.svc.StartGeneration(t.Context(), ws.WorksetID, light.kind, workset.StartGenerationRequest{
		IfMatchVersion: f.operationFor(ws.WorksetID, light.kind).Version, IdempotencyKey: "stop-light",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitPlanner(t, light.entered)
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	if err = handle.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{heavyID, waitingID, result.Generation.GenerationID} {
		g, getErr := f.repo.GetGeneration(id)
		if getErr != nil || g.Status != workset.GenStatusInterrupted {
			t.Fatalf("Stop failed to interrupt %s: %+v %v", id, g, getErr)
		}
	}
}

func TestGeneralWorkerStillClaimsLightPlanning(t *testing.T) {
	f := newFixture(t)
	heavy := newGatedPlanner("heavy")
	heavy.release = make(chan struct{})
	light := lightTestPlanner{newGatedPlanner("light")}
	second := lightTestPlanner{newGatedPlanner("second")}
	f.svc = workset.NewService(f.repo, 1, 1, []workset.Task{heavy, light, second}, nil, nil)
	ids := f.standardLibrary("albumA")
	ws := f.createCurrentOperation("lib-1", heavy.kind, ids...)
	enqueuePlanner(t, f, ws.WorksetID, heavy.kind)
	handle := f.svc.DispatcherHandle()
	handle.Start()
	t.Cleanup(func() { _ = handle.Stop(context.Background()) })
	waitPlanner(t, heavy.entered)
	enqueuePlanner(t, f, ws.WorksetID, light.kind)
	waitPlanner(t, light.entered)
	enqueuePlanner(t, f, ws.WorksetID, second.kind)
	close(heavy.release)
	waitPlanner(t, second.entered)
	if err := handle.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestGenerationClaimFiltersKinds(t *testing.T) {
	f := newFixture(t)
	heavy := stubTask{kind: "heavy", seed: `{}`}
	light := stubTask{kind: "light", seed: `{}`}
	workset.RegisterTasksForTest(f.svc, []workset.Task{heavy, light})
	ids := f.standardLibrary("albumA")
	ws := f.createCurrentOperation("lib-1", heavy.kind, ids...)
	heavyID := enqueuePlanner(t, f, ws.WorksetID, heavy.kind)
	lightID := enqueuePlanner(t, f, ws.WorksetID, light.kind)
	if g, err := f.repo.NextQueuedGeneration([]string{}); err != nil || g != nil {
		t.Fatalf("empty filter claimed a task: %+v %v", g, err)
	}
	if g, err := f.repo.NextQueuedGeneration(
		[]string{"absent", light.kind},
	); err != nil || g == nil ||
		g.GenerationID != lightID {
		t.Fatalf("kind filter claimed wrong task: %+v %v", g, err)
	}
	if g, err := f.repo.NextQueuedGeneration(nil); err != nil || g == nil || g.GenerationID != heavyID {
		t.Fatalf("general FIFO lost queued task: %+v %v", g, err)
	}
}
