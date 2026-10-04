package refresh_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/onsei/organizer/backend/internal/works/refresh"
	"github.com/onsei/organizer/backend/internal/workset"
)

func TestSelectedMetadataExecutionIgnoresMissingOtherRootAndDoesNotRecheck(t *testing.T) {
	run := func(_ context.Context, in workset.BatchExecutionInput) error {
		if len(in.Units) != 1 {
			t.Fatalf("scope: %+v", in.Units)
		}
		// Simulate a directory move after the precheck. No post-execution scan
		// or directory revalidation should change the persisted batch result.
		if err := os.Rename(in.Units[0].RootPath, in.Units[0].RootPath+"-moved"); err != nil {
			return err
		}
		done := 1
		return in.Progress(in.Units[0].Index, workset.UnitResult{CompletedOperations: &done})
	}
	svc, _, _, root, names, libID, _ := worksetFixture(t, run)
	record, err := svc.CreateCurrentWorkset(
		t.Context(),
		workset.CreateCurrentRequest{LibraryID: libID, OperationType: refresh.Kind, FolderPaths: names},
	)
	if err != nil {
		t.Fatal(err)
	}
	id := record.Workset.WorksetID
	gen, err := svc.StartGeneration(
		t.Context(),
		id,
		refresh.Kind,
		workset.StartGenerationRequest{IfMatchVersion: 1, IdempotencyKey: "light-plan"},
	)
	if err != nil {
		t.Fatal(err)
	}
	svc.DispatcherHandle().Start()
	defer svc.DispatcherHandle().Stop()
	plan := waitGeneration(t, svc, id, gen.Generation.GenerationID)
	op, err := svc.GetOperation(t.Context(), id, refresh.Kind)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(root, names[1])); err != nil {
		t.Fatal(err)
	}
	_, err = svc.StartExecution(
		t.Context(),
		id,
		refresh.Kind,
		plan.RevisionID,
		workset.StartExecutionRequest{IfMatchVersion: op.Version, IdempotencyKey: "full"},
	)
	if e, ok := workset.AsError(err); !ok || !slices.Contains(e.Details, "INPUT_CHANGED") {
		t.Fatalf("full execution missing root: %v", err)
	}
	started, err := svc.StartExecution(
		t.Context(),
		id,
		refresh.Kind,
		plan.RevisionID,
		workset.StartExecutionRequest{IfMatchVersion: op.Version, IdempotencyKey: "selected", FolderPaths: names[:1]},
	)
	if err != nil {
		t.Fatal(err)
	}
	result := waitExecution(t, svc, id, started.Execution.ExecutionID)
	if result.Status != "succeeded" || result.CompletedComponents != 1 {
		t.Fatalf("selected execution: %+v", result)
	}
}
