package sqlite_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func batchExecutionRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	repo := newExecutionRepo(t)
	seedExecutionWorkset(t, repo, "ws-1", "/music")
	if err := createExecution(t, repo, newExecution("exec-1", "ws-1", "plan-1", "key-1"), "plan-1"); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestSaveExecutionComponentResultsMatchesSingleWritesAndReplays(t *testing.T) {
	batch, single := batchExecutionRepo(t), batchExecutionRepo(t)
	position := workset.ExecutionPosition{CurrentComponentIndex: 3, CurrentPhase: "component"}
	batches := [][]workset.ExecutionComponentResult{
		{
			{ComponentIndex: 0, Status: "succeeded", ResultJSON: `{"stage":"skipped"}`},
			{ComponentIndex: 1, Status: "pending", CompletedOperations: 2, ResultJSON: `{}`},
		},
		{
			{ComponentIndex: 0, Status: "pending", ResultJSON: `{}`},
			{ComponentIndex: 1, Status: "succeeded", CompletedOperations: 4, ResultJSON: `{}`},
			{ComponentIndex: 2, Status: "failed", CompletedOperations: 1, ResultJSON: `{}`},
			{ComponentIndex: 2, Status: "succeeded", CompletedOperations: 3, ResultJSON: `{}`},
		},
	}
	for _, results := range batches {
		for range 2 {
			if err := batch.SaveExecutionComponentResults("exec-1", results, position); err != nil {
				t.Fatal(err)
			}
			for _, result := range results {
				if err := single.SaveExecutionComponentResult("exec-1", result, position); err != nil {
					t.Fatal(err)
				}
			}
			assertBatchMatchesSingles(t, batch, single, position)
		}
	}
}

func assertBatchMatchesSingles(
	t *testing.T, batch, single *sqlite.Repository, position workset.ExecutionPosition,
) {
	t.Helper()
	got, err := batch.GetExecution("exec-1")
	if err != nil {
		t.Fatal(err)
	}
	want, err := single.GetExecution("exec-1")
	if err != nil {
		t.Fatal(err)
	}
	var components, operations int
	if err = batch.DB().QueryRow(`SELECT COUNT(*) FILTER (WHERE status <> 'pending'),
		COALESCE(SUM(completed_operations), 0) FROM execution_component_results
		WHERE execution_id = 'exec-1'`).Scan(&components, &operations); err != nil {
		t.Fatal(err)
	}
	if got.CompletedComponents != want.CompletedComponents || got.CompletedOperations != want.CompletedOperations ||
		got.CompletedComponents != components || got.CompletedOperations != operations ||
		got.CurrentComponentIndex != position.CurrentComponentIndex || got.CurrentPhase != position.CurrentPhase {
		t.Fatalf("batch progress differs from single writes or recount: got %+v, want %+v, recount %d/%d",
			got, want, components, operations)
	}
	gotRows, err := batch.ListExecutionComponentResults("exec-1", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantRows, err := single.ListExecutionComponentResults("exec-1", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := range gotRows {
		gotRows[i].UpdatedAt = time.Time{}
	}
	for i := range wantRows {
		wantRows[i].UpdatedAt = time.Time{}
	}
	if !reflect.DeepEqual(gotRows, wantRows) {
		t.Fatalf("batch rows %+v differ from single writes %+v", gotRows, wantRows)
	}
}

func TestSaveExecutionComponentResultsRollsBackRowsAndCounters(t *testing.T) {
	for name, trigger := range map[string]string{
		"second row": `CREATE TRIGGER fail_batch BEFORE INSERT ON execution_component_results
			WHEN NEW.component_index = 1 BEGIN SELECT RAISE(FAIL, 'row blocked'); END`,
		"counters": `CREATE TRIGGER fail_batch BEFORE UPDATE ON plan_executions
			BEGIN SELECT RAISE(FAIL, 'progress blocked'); END`,
	} {
		t.Run(name, func(t *testing.T) {
			repo := batchExecutionRepo(t)
			first := workset.ExecutionComponentResult{
				ComponentIndex: 0, Status: "succeeded", CompletedOperations: 3, ResultJSON: `{"original":true}`,
			}
			position := workset.ExecutionPosition{CurrentComponentIndex: 1}
			if err := repo.SaveExecutionComponentResult("exec-1", first, position); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.DB().Exec(trigger); err != nil {
				t.Fatal(err)
			}
			err := repo.SaveExecutionComponentResults("exec-1", []workset.ExecutionComponentResult{
				{ComponentIndex: 0, Status: "pending", ResultJSON: `{}`},
				{ComponentIndex: 1, Status: "succeeded", ResultJSON: `{"stage":"skipped"}`},
			}, workset.ExecutionPosition{})
			if err == nil {
				t.Fatal("batch failure ignored")
			}
			rows, err := repo.ListExecutionComponentResults("exec-1", 0, 0)
			if err != nil || len(rows) != 1 || rows[0].Status != first.Status ||
				rows[0].ResultJSON != first.ResultJSON ||
				rows[0].CompletedOperations != first.CompletedOperations {
				t.Fatalf("failed batch changed rows: %+v, %v", rows, err)
			}
			progress, err := repo.GetExecution("exec-1")
			if err != nil || progress.CompletedComponents != 1 || progress.CompletedOperations != 3 ||
				progress.CurrentComponentIndex != position.CurrentComponentIndex {
				t.Fatalf("failed batch changed progress: %+v, %v", progress, err)
			}
		})
	}
}
