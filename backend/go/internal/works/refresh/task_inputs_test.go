package refresh_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestDirectoryRulesMatchAdmissionAndWorkerScope(t *testing.T) {
	root := t.TempDir()
	selected, other := filepath.Join(root, "selected"), filepath.Join(root, "other")
	if err := os.Mkdir(selected, 0755); err != nil {
		t.Fatal(err)
	}
	task := refresh.NewTask(taskCatalog{}, filesystem.CheckWorkDirectory)
	facts := workset.RevisionFacts{Detail: &workset.PlanDetail{
		Plan: workset.Plan{TaskSchemaVersion: 1},
		Steps: []workset.PlanStepRecord{
			{PolicySchemaVersion: 1, PolicyJSON: `{"mode":"fill_missing","locale":"ja-jp","download_cover":true}`},
		},
		Roots: []workset.PlanRootRecord{{RootIndex: 0, RootPath: selected}, {RootIndex: 1, RootPath: other}},
	}}
	facts.InputRoots = []string{selected}
	health, err := task.EvaluateRevision(context.Background(), facts)
	if err != nil || health.InputMoved {
		t.Fatalf("selected admission: %+v %v", health, err)
	}
	_, err = task.NewExecutionSession(
		t.Context(),
		workset.ExecutionSessionInput{Units: []workset.ExecutionUnit{{RootPath: selected}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	// Replacing the physical object with the same directory name remains valid.
	if err = os.Rename(selected, selected+"-old"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(selected, 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(selected, "unreadable.wav"), []byte("changed media"), 0000); err != nil {
		t.Fatal(err)
	}
	health, err = task.EvaluateRevision(context.Background(), facts)
	if err != nil || health.InputMoved {
		t.Fatalf("replacement: %+v %v", health, err)
	}
	facts.InputRoots = nil
	health, err = task.EvaluateRevision(context.Background(), facts)
	if err != nil || !health.InputMoved || len(health.StaleRoots) != 1 || health.StaleRoots[0] != 1 {
		t.Fatalf("full admission: %+v %v", health, err)
	}
	_, err = task.NewExecutionSession(
		t.Context(),
		workset.ExecutionSessionInput{Units: []workset.ExecutionUnit{{RootPath: other}}},
	)
	if e, ok := workset.AsError(err); !ok || e.Code != "INPUT_CHANGED" {
		t.Fatalf("missing worker: %v", err)
	}
	if err = os.RemoveAll(selected); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(selected, nil, 0600); err != nil {
		t.Fatal(err)
	}
	facts.InputRoots = []string{selected}
	health, err = task.EvaluateRevision(context.Background(), facts)
	if err != nil || !health.InputMoved {
		t.Fatalf("non-directory: %+v %v", health, err)
	}
}

func TestMetadataEmptyScopeAndCancellationDoNotCheckDirectories(t *testing.T) {
	checks := 0
	task := refresh.NewTask(taskCatalog{}, func(string) error { checks++; return fs.ErrPermission })
	_, err := task.NewExecutionSession(t.Context(), workset.ExecutionSessionInput{})
	if err != nil || checks != 0 {
		t.Fatalf("empty: %d %v", checks, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = task.NewExecutionSession(
		ctx,
		workset.ExecutionSessionInput{Units: []workset.ExecutionUnit{{RootPath: "one"}}},
	)
	if !errors.Is(err, context.Canceled) || checks != 0 {
		t.Fatalf("canceled: %d %v", checks, err)
	}
	_, err = task.NewExecutionSession(
		t.Context(),
		workset.ExecutionSessionInput{Units: []workset.ExecutionUnit{{RootPath: "one"}}},
	)
	if e, ok := workset.AsError(err); !ok || e.Code != "SCAN_FAILED" || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("I/O diagnostic: %v", err)
	}
	ctx, cancel = context.WithCancel(t.Context())
	task = refresh.NewTask(taskCatalog{}, func(string) error { cancel(); return fs.ErrPermission })
	_, err = task.NewExecutionSession(
		ctx,
		workset.ExecutionSessionInput{Units: []workset.ExecutionUnit{{RootPath: "one"}}},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation precedence: %v", err)
	}
}
