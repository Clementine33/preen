package workset_test

import (
	"context"
	"testing"

	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestBatchProgressKeepsCommittedResultsAndSettlesUnrunUnits(t *testing.T) {
	for _, scenario := range []string{
		"success", "write failure", "cancel before batch", "cancel after batch", "cancel before single result",
	} {
		t.Run(scenario, func(t *testing.T) {
			var f *execFixture
			batch := func(ctx context.Context, in workset.BatchExecutionInput) error {
				return reportBatchScenario(t, ctx, in, f, scenario)
			}
			task := newPoolTask(newPoolObservations(), poolUnit(0), poolUnit(0), poolUnit(0))
			f = newExecFixtureWithTask(t, nil, 1, []workset.Task{task},
				map[string]workset.BatchExecutor{workset.OperationTypeConversion: batch})
			f.seedRevision("plan-batch", poolComponent("u0", "albumA"),
				poolComponent("u1", "albumA"), poolComponent("u2", "albumB"))
			if scenario == "write failure" {
				if _, err := f.repo.DB().Exec(`CREATE TRIGGER fail_skipped_result
					BEFORE INSERT ON execution_component_results
					WHEN json_extract(NEW.result_json, '$.stage') = 'skipped'
					BEGIN SELECT RAISE(FAIL, 'batch write blocked'); END`); err != nil {
					t.Fatal(err)
				}
			}
			f.runDispatcher()
			started := f.mustStart("plan-batch", "k-batch")
			view := f.waitTerminal(started.ExecutionID)
			assertBatchScenario(t, view, scenario)
		})
	}
}

func reportBatchScenario(
	t *testing.T, ctx context.Context, in workset.BatchExecutionInput, f *execFixture, scenario string,
) error {
	t.Helper()
	cancel := func() {
		if _, err := f.svc.CancelExecution(
			ctx,
			f.worksetID,
			workset.OperationTypeConversion,
			in.ExecutionID,
		); err != nil {
			t.Error(err)
		}
		<-ctx.Done()
	}
	if scenario == "cancel before batch" {
		cancel()
	}
	zero := 0
	result := workset.UnitResult{
		Stage: "skipped", CompletedOperations: &zero, TaskResult: []byte(`{"action":"skip"}`),
	}
	if scenario == "cancel before single result" {
		cancel()
		if err := in.Progress(in.Units[0].Index, result); err != nil {
			return err
		}
		return ctx.Err()
	}
	if err := in.ProgressBatch(map[int]workset.UnitResult{
		in.Units[0].Index: result, in.Units[1].Index: result,
	}); err != nil {
		return err
	}
	if scenario == "cancel after batch" {
		cancel()
		return ctx.Err()
	}
	return in.Progress(in.Units[2].Index, workset.UnitResult{CompletedOperations: &zero})
}

func assertBatchScenario(t *testing.T, view *workset.ExecutionView, scenario string) {
	t.Helper()
	wantStatus, wantCode := workset.ExecStatusSucceeded, ""
	if scenario == "write failure" {
		wantStatus, wantCode = workset.ExecStatusFailed, "RESULT_PERSIST_FAILED"
	} else if scenario != "success" {
		wantStatus, wantCode = workset.ExecStatusCanceled, "CANCELED"
	}
	if view.Status != wantStatus || view.ErrorCode != wantCode ||
		view.CompletedComponents != 3 || view.CompletedOperations != 0 || len(view.Components) != 3 {
		t.Fatalf("batch outcome: %+v", view)
	}
	for i, entry := range view.Components {
		committed := scenario == "success" || scenario == "cancel after batch" && i < 2 ||
			scenario == "cancel before single result" && i == 0
		if committed {
			if entry.Status != workset.ExecComponentSucceeded || !entry.InventorySynced || entry.ErrorCode != "" {
				t.Fatalf("committed result changed: %+v", entry)
			}
			if i < 2 && (entry.Stage != "skipped" || string(entry.TaskResult) != `{"action":"skip"}`) {
				t.Fatalf("skip facts changed: %+v", entry)
			}
		} else if entry.Status != wantStatus || entry.ErrorCode != wantCode || entry.Stage != "" {
			t.Fatalf("uncommitted unit escaped teardown: %+v", entry)
		}
	}
}
