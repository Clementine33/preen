package refresh_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/works/refresh"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestMetadataShutdownKeepsInterruptionAndFailureDistinct(t *testing.T) {
	for _, outcome := range []string{"interrupted", "error", "failed_result"} {
		t.Run(outcome, func(t *testing.T) {
			entered := make(chan struct{})
			run := func(ctx context.Context, in workset.BatchExecutionInput) error {
				if outcome == "failed_result" {
					if err := in.Progress(
						in.Units[0].Index,
						workset.UnitResult{ErrorCode: "UPSTREAM_FAILED"},
					); err != nil {
						return err
					}
				}
				close(entered)
				<-ctx.Done()
				if outcome == "error" {
					return errors.New("independent metadata failure")
				}
				return ctx.Err()
			}
			svc, _, _, _, _, libraryID, _, _ := worksetFixture(t, run)
			record, err := svc.CreateCurrentWorkset(t.Context(), workset.CreateCurrentRequest{
				LibraryID: libraryID, OperationType: refresh.Kind, Selection: workset.Selection{All: true},
			})
			if err != nil {
				t.Fatal(err)
			}
			id := record.Workset.WorksetID
			generation, err := svc.StartGeneration(t.Context(), id, refresh.Kind, workset.StartGenerationRequest{
				IfMatchVersion: 1, IdempotencyKey: "shutdown-plan",
			})
			if err != nil {
				t.Fatal(err)
			}
			svc.DispatcherHandle().Start()
			defer stopDispatcher(t, svc)
			plan := waitGeneration(t, svc, id, generation.Generation.GenerationID)
			op, err := svc.GetOperation(t.Context(), id, refresh.Kind)
			if err != nil {
				t.Fatal(err)
			}
			execution, err := svc.StartExecution(
				t.Context(),
				id,
				refresh.Kind,
				plan.RevisionID,
				workset.StartExecutionRequest{
					IfMatchVersion: op.Version, IdempotencyKey: "shutdown-run",
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(4 * time.Second):
				t.Fatal("metadata execution did not start")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
			defer cancel()
			if err := svc.DispatcherHandle().Stop(ctx); err != nil {
				t.Fatal(err)
			}
			view := waitExecution(t, svc, id, execution.Execution.ExecutionID)
			want := workset.ExecStatusInterrupted
			if outcome != "interrupted" {
				want = workset.ExecStatusFailed
			}
			if view.Status != want || view.ErrorCode == "CANCELED" {
				t.Fatalf("execution = %+v", view)
			}
		})
	}
}
