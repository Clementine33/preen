package dirrename

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/onsei/organizer/backend/internal/workset"
)

// InterruptedExecution keeps the original worklist authoritative during repair.
type InterruptedExecution struct {
	ID          string
	Root        string
	RequestJSON string
}

// RecoveryStore reads interrupted requests and persists recovered component facts.
type RecoveryStore interface {
	InterruptedRenames(context.Context) ([]InterruptedExecution, error)
	ListExecutionComponentResults(string, int, int) ([]workset.ExecutionComponentResult, error)
	SaveExecutionComponentResult(string, workset.ExecutionComponentResult, workset.ExecutionPosition) error
}

// RecoverInterrupted repairs only the first missing result of each interrupted
// run. It never resumes execution, scans media or revisits persisted failures.
func (t *Task) RecoverInterrupted(ctx context.Context, store RecoveryStore) []error {
	executions, err := store.InterruptedRenames(ctx)
	if err != nil {
		return []error{err}
	}
	var diagnostics []error
	for _, ex := range executions {
		if err := t.recoverExecution(ctx, store, ex); err != nil {
			diagnostics = append(diagnostics, fmt.Errorf("rename %s: %w", ex.ID, err))
		}
	}
	return diagnostics
}
func (t *Task) recoverExecution(ctx context.Context, store RecoveryStore, ex InterruptedExecution) error {
	var req struct {
		Units []workset.ExecutionUnit `json:"units"`
	}
	if err := json.Unmarshal([]byte(ex.RequestJSON), &req); err != nil {
		return err
	}
	rows, err := store.ListExecutionComponentResults(ex.ID, 0, 0)
	if err != nil {
		return err
	}
	saved := map[int]bool{}
	for _, row := range rows {
		saved[row.ComponentIndex] = true
	}
	for _, unit := range req.Units {
		if saved[unit.Index] {
			continue
		}
		return t.recoverUnit(ctx, store, ex, unit)
	}
	return nil
}

func (t *Task) recoverUnit(
	ctx context.Context,
	store RecoveryStore,
	ex InterruptedExecution,
	u workset.ExecutionUnit,
) error {
	p, err := t.prepareRename(ctx, workset.UnitRunInput{WorksetRoot: ex.Root, Unit: u})
	if err != nil {
		return err
	}
	if p.action != "rename" {
		return nil
	}
	_, sourceErr := os.Lstat(filepath.FromSlash(p.result.SourcePath))
	targetInfo, targetErr := os.Lstat(filepath.FromSlash(p.result.TargetPath))
	if sourceErr == nil && os.IsNotExist(targetErr) {
		return nil
	} // Not started.
	if sourceErr != nil && !os.IsNotExist(sourceErr) {
		return sourceErr
	}
	if targetErr != nil {
		return fmt.Errorf("cannot repair target: %w", targetErr)
	}
	if sourceErr == nil {
		return fmt.Errorf("source and target both exist")
	}
	if !targetInfo.IsDir() || targetInfo.Name() != filepath.Base(filepath.FromSlash(p.result.TargetPath)) {
		return fmt.Errorf("repair target is not the frozen directory")
	}
	if t.io.ResolveDirectory == nil || t.io.Relocate == nil {
		return errors.New("rename recovery is not configured")
	}
	rel, err := filepath.Rel(filepath.FromSlash(ex.Root), filepath.FromSlash(p.result.TargetPath))
	if err != nil {
		return err
	}
	if _, err = t.io.ResolveDirectory(ex.Root, filepath.ToSlash(rel)); err != nil {
		return err
	}
	if err = t.io.Relocate(ctx, ex.Root, p.recorded, p.result.TargetPath, p.rebind); err != nil {
		return err
	}
	p.result.Renamed, p.result.Recovered = true, true
	facts, err := json.Marshal(struct {
		TaskResult      Result   `json:"task_result"`
		Committed       []string `json:"committed"`
		Removed         []string `json:"removed"`
		InventorySynced bool     `json:"inventory_synced"`
	}{p.result, []string{p.result.TargetPath}, []string{p.result.SourcePath}, true})
	if err != nil {
		return err
	}
	return store.SaveExecutionComponentResult(ex.ID, workset.ExecutionComponentResult{
		ComponentIndex:      u.Index,
		Status:              workset.ExecComponentSucceeded,
		CompletedOperations: 1,
		ResultJSON:          string(facts),
	}, workset.ExecutionPosition{})
}
