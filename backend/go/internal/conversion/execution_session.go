package conversion

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"sync"

	"github.com/onsei/organizer/backend/internal/conversion/execute"
	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/workset"
)

// ExecutionIO supplies conversion's read-only walk and strict move mechanism.
type ExecutionIO struct {
	Walk  SelectedWalk
	Moves execute.FileMoves
	Scan  SelectedScan
	Files execute.FileHandles
}

// SelectedWalk reads selected folder subtrees without persisting inventory.
type SelectedWalk func(context.Context, []string, string, func(string, inventory.DirEntry) error) error

// NewExecutionSession verifies all execution roots before preparing any unit.
// Successful checks read disk facts only; drift refresh is diagnostic and never
// authorizes execution of the old revision.
func (t *Task) NewExecutionSession(
	ctx context.Context,
	in workset.ExecutionSessionInput,
) (workset.UnitPreparer, error) {
	if ctx.Err() != nil {
		return nil, workset.NewError(workset.ErrKindInternal, "CANCELED", "execution canceled", ctx.Err())
	}
	moves, specs, requirementsErr := executionRequirements(in)
	if requirementsErr != nil {
		return nil, workset.NewError(
			workset.ErrKindInternal,
			"REQUEST_LOAD_FAILED",
			"frozen execution requirements are unreadable",
			requirementsErr,
		)
	}
	var config execute.ToolsConfig
	if len(specs) > 0 {
		config = t.settings.Tools() // Capture before walking; later edits belong to the next session.
	}
	roots := []string{}
	seen := map[string]bool{}
	for _, unit := range in.Units {
		if !seen[unit.RootPath] {
			roots = append(roots, unit.RootPath)
			seen[unit.RootPath] = true
		}
	}
	if len(roots) > 0 {
		if err := t.verifyExecutionInputs(ctx, in, roots); err != nil {
			return nil, err
		}
	}
	return t.configureExecution(ctx, moves, specs, config)
}

func (t *Task) verifyExecutionInputs(ctx context.Context, in workset.ExecutionSessionInput, roots []string) error {
	if t.executionWalk == nil {
		return workset.NewError(workset.ErrKindInternal, "SCAN_FAILED", "execution walker is unavailable", nil)
	}
	entries := map[string][]reconcile.AudioEntry{}
	var mu sync.Mutex
	err := t.executionWalk(ctx, roots, in.WorksetRoot, func(root string, e inventory.DirEntry) error {
		if !e.IsDir {
			mu.Lock()
			entries[root] = append(
				entries[root],
				reconcile.AudioEntry{PathPosix: filepath.ToSlash(e.Path), Size: e.Size, Mtime: e.Mtime},
			)
			mu.Unlock()
		}
		return nil
	})
	if ctx.Err() != nil {
		return workset.NewError(workset.ErrKindInternal, "CANCELED", "execution canceled", ctx.Err())
	}
	changed := errors.Is(err, fs.ErrNotExist)
	if err != nil && !changed {
		return workset.NewError(
			workset.ErrKindInternal,
			"SCAN_FAILED",
			"failed to walk execution inputs: "+err.Error(),
			err,
		)
	}
	matched := 0
	for _, root := range in.Detail.Roots {
		if !slices.Contains(roots, root.RootPath) {
			continue
		}
		matched++
		digest, count := reconcile.InventoryFingerprint(reconcile.AudioEntries(entries[root.RootPath]))
		changed = changed || root.RootStatus == "missing" || digest != root.InventoryFingerprint ||
			count != root.EntryCount
	}
	changed = changed || matched != len(roots)
	if !changed {
		return nil
	}
	message := "execution inputs changed; regenerate the plan"
	if in.Refresh != nil {
		if refreshErr := in.Refresh(ctx, roots); refreshErr != nil {
			message += fmt.Sprintf("; inventory refresh failed: %v", refreshErr)
		}
	}
	return workset.NewError(workset.ErrKindConflict, "INPUT_CHANGED", message, err)
}
