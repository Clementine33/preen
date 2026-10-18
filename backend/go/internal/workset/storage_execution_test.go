package workset_test

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/library"
	"github.com/Clementine33/preen/backend/internal/workset"
)

type storageSessionTask struct {
	*poolTask

	entered chan struct{}
	proceed chan struct{}
}

func (s *storageSessionTask) NewExecutionSession(
	ctx context.Context,
	_ workset.ExecutionSessionInput,
) (workset.UnitPreparer, error) {
	s.entered <- struct{}{}
	select {
	case <-s.proceed:
		return s, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestExecutionFreezesLibraryStorageBeforeSessionPreparation(t *testing.T) {
	obs := newPoolObservations()
	workers := library.StorageSSD.EncodeWorkers(runtime.NumCPU())
	task := &storageSessionTask{newPoolTask(obs, poolUnit(workers+3)), make(chan struct{}, 1), make(chan struct{}, 1)}
	f := newExecFixtureWithTask(t, nil, 0, []workset.Task{task})
	libs := library.NewService(f.repo, nil, f.repo, nil)
	class := library.StorageSSD
	if _, err := libs.Patch(f.libraryID, nil, nil, &class); err != nil {
		t.Fatal(err)
	}
	f.runDispatcher()
	for i, want := range []int{workers, library.StorageHDD.EncodeWorkers(runtime.NumCPU())} {
		id := fmt.Sprintf("storage-%d", i)
		obs.gate("u0")
		f.seedRevision(id, poolComponent("u0", "albumA"))
		run := f.mustStart(id, id)
		select {
		case <-task.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("session did not start")
		}
		// The first session already started, including its preflight. Its width is frozen.
		class = library.StorageHDD
		if _, err := libs.Patch(f.libraryID, nil, nil, &class); err != nil {
			t.Fatal(err)
		}
		task.proceed <- struct{}{}
		deadline := time.Now().Add(5 * time.Second)
		for {
			obs.mu.Lock()
			running := obs.running
			obs.mu.Unlock()
			if running == want {
				break
			}
			if time.Now().After(deadline) {
				obs.release("u0")
				t.Fatalf("active encodes=%d, want %d", running, want)
			}
			time.Sleep(time.Millisecond)
		}
		obs.release("u0")
		if done := f.waitTerminal(run.ExecutionID); done.Status != "succeeded" {
			t.Fatalf("session = %+v", done)
		}
		obs.mu.Lock()
		peak := obs.maxRunning
		obs.maxRunning = 0
		obs.mu.Unlock()
		if peak != want {
			t.Fatalf("peak encodes=%d, want %d", peak, want)
		}
	}
}
