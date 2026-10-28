package refresh_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Clementine33/preen/backend/internal/works/refresh"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestMetadataWorkersAutomaticallyRefreshOnlyExecutedScope(t *testing.T) {
	var calls atomic.Int32
	svc, covers, _, _, names, libID, _ := worksetFixtureWithRefresh(
		t, func(context.Context, string, []string) error {
			calls.Add(1)
			return nil
		},
	)
	enableCoverDownload(t, svc)
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
	if worked, err := covers.RefreshNext(t.Context()); !worked || err != nil {
		t.Fatalf("canceled covers did not finish follow-up: %v %v", worked, err)
	}
	if worked, err := covers.RefreshNext(t.Context()); worked || err != nil || calls.Load() != 0 {
		t.Fatalf("canceled covers scanned inventory: %v %v, calls %d", worked, err, calls.Load())
	}
}
