package refresh_test

import (
	"context"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/onsei/organizer/backend/internal/works/refresh"
	"github.com/onsei/organizer/backend/internal/workset"
)

func TestMetadataWorkersAutomaticallyRefreshOnlyExecutedScope(t *testing.T) {
	type scope struct {
		root    string
		folders []string
	}
	refreshed := make(chan scope, 1)
	var calls atomic.Int32
	svc, covers, _, root, names, libID, _ := worksetFixtureWithRefresh(
		t, func(_ context.Context, root string, folders []string) error {
			calls.Add(1)
			refreshed <- scope{root, slices.Clone(folders)}
			return nil
		},
	)
	record, err := svc.CreateCurrentWorkset(t.Context(), workset.CreateCurrentRequest{
		LibraryID: libID, OperationType: refresh.Kind, FolderPaths: names,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := record.Workset.WorksetID
	gen, err := svc.StartGeneration(t.Context(), id, refresh.Kind,
		workset.StartGenerationRequest{IfMatchVersion: 1, IdempotencyKey: "selected-refresh"})
	if err != nil {
		t.Fatal(err)
	}
	svc.DispatcherHandle().Start()
	defer stopDispatcher(t, svc)
	plan := waitGeneration(t, svc, id, gen.Generation.GenerationID)
	op, err := svc.GetOperation(t.Context(), id, refresh.Kind)
	if err != nil {
		t.Fatal(err)
	}
	started, err := svc.StartExecution(t.Context(), id, refresh.Kind, plan.RevisionID,
		workset.StartExecutionRequest{IfMatchVersion: op.Version, IdempotencyKey: "run", FolderPaths: names[:1]})
	if err != nil {
		t.Fatal(err)
	}
	result := waitExecution(t, svc, id, started.Execution.ExecutionID)
	if result.Status != "succeeded" {
		t.Fatalf("metadata: %+v", result)
	}
	if worked, err := covers.RefreshNext(t.Context()); worked || err != nil {
		t.Fatalf("refreshed before cover queue ended: %v %v", worked, err)
	}
	if err := covers.Cancel(t.Context(), result.ExecutionID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- covers.Run(ctx) }()
	defer func() { cancel(); <-done }()
	select {
	case got := <-refreshed:
		want := []string{filepath.ToSlash(filepath.Join(root, names[0]))}
		if got.root != filepath.ToSlash(root) || !slices.Equal(got.folders, want) {
			t.Fatalf("follow-up scope: %+v, want %s %v", got, root, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("workers did not run the completion refresh")
	}
	if worked, err := covers.RefreshNext(t.Context()); worked || err != nil || calls.Load() != 1 {
		t.Fatalf("duplicate refresh: %v %v, calls %d", worked, err, calls.Load())
	}
}
