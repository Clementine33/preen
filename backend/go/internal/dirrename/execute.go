package dirrename

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// Result states the observed location independently of database sync success.
type Result struct {
	SourcePath string `json:"source_path"`
	TargetPath string `json:"target_path"`
	Renamed    bool   `json:"renamed"`
	RolledBack bool   `json:"rolled_back,omitempty"`
	Recovered  bool   `json:"recovered,omitempty"`
	// Skipped is the reason a blocked folder was left alone; the run goes on.
	Skipped string `json:"skipped,omitempty"`
}

func (t *Task) FreezeExecution(
	ctx context.Context,
	in workset.RevisionFacts,
) (workset.FrozenExecution, []string, error) {
	frozen := workset.FrozenExecution{Units: []workset.ExecutionUnit{}}
	if err := validateRevision(in); err != nil {
		return frozen, nil, err
	}
	roots := map[int]string{}
	for _, r := range in.Detail.Roots {
		roots[r.RootIndex] = r.RootPath
	}
	for _, c := range in.Detail.Components {
		var o Outcome
		if err := json.Unmarshal([]byte(c.OutcomeJSON), &o); err != nil {
			return frozen, nil, err
		}
		operations := 0
		if o.Action == "rename" {
			operations = 1
		}
		frozen.Units = append(frozen.Units, workset.ExecutionUnit{
			Index: c.ComponentIndex, RootIndex: c.RootIndex, ID: c.ComponentID,
			RootPath: roots[c.RootIndex], Operations: operations, Payload: json.RawMessage(c.OutcomeJSON),
		})
		frozen.TotalOperations += operations
	}
	var selected []workset.ExecutionUnit
	for _, u := range frozen.Units {
		if in.InputRoots == nil || slices.Contains(in.InputRoots, u.RootPath) {
			selected = append(selected, u)
		}
	}
	if err := t.checkCovers(ctx, in.Detail.Plan.RootPath, selected); err != nil {
		return frozen, nil, err
	}
	return frozen, nil, nil
}

func (t *Task) checkCovers(ctx context.Context, root string, units []workset.ExecutionUnit) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var paths []string
	for _, u := range units {
		if u.Operations > 0 {
			paths = append(paths, u.RootPath)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	if t.io.PendingCovers == nil {
		return fmt.Errorf("rename cover admission is not configured")
	}
	pending, err := t.io.PendingCovers(ctx, root, paths)
	if err != nil {
		return err
	}
	if pending {
		return workset.NewError(
			workset.ErrKindConflict,
			"COVER_DELIVERY_PENDING",
			"wait for or cancel cover deliveries before renaming",
			nil,
		)
	}
	return nil
}

func (t *Task) NewExecutionSession(
	ctx context.Context,
	in workset.ExecutionSessionInput,
) (workset.UnitPreparer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(in.Units) == 0 {
		return t, nil
	}
	paths := make([]string, 0, len(in.Units))
	for _, u := range in.Units {
		paths = append(paths, u.RootPath)
	}
	health, err := t.EvaluateRevision(ctx, workset.RevisionFacts{Detail: in.Detail, InputRoots: paths})
	if canceled := ctx.Err(); canceled != nil {
		return nil, canceled
	}
	if err != nil {
		return nil, workset.NewError(workset.ErrKindInternal, "SCAN_FAILED", "failed to check rename inputs", err)
	}
	if health.InputMoved {
		return nil, workset.NewError(
			workset.ErrKindConflict,
			"INPUT_CHANGED",
			"rename inputs changed since planning",
			nil,
		)
	}
	if err := t.checkCovers(ctx, in.WorksetRoot, in.Units); err != nil {
		return nil, err
	}
	return t, ctx.Err()
}

func (t *Task) PrepareUnit(ctx context.Context, in workset.UnitRunInput) (workset.PreparedUnit, error) {
	return t.prepareRename(ctx, in)
}

func (t *Task) prepareRename(ctx context.Context, in workset.UnitRunInput) (*preparedRename, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var o Outcome
	if err := json.Unmarshal(in.Unit.Payload, &o); err != nil {
		return nil, err
	}
	if o.Action == "blocked" {
		return &preparedRename{
			task: t, root: in.WorksetRoot, action: o.Action, recorded: in.Unit.RootPath,
			result: Result{SourcePath: in.Unit.RootPath, Skipped: o.ReasonCode},
		}, nil
	}
	if o.Action != "rename" && o.Action != "noop" {
		return nil, fmt.Errorf("invalid frozen rename action %q", o.Action)
	}
	if filepath.Base(filepath.FromSlash(in.Unit.RootPath)) != o.SourceName ||
		o.TargetName == "" || filepath.Base(o.TargetName) != o.TargetName || strings.ContainsAny(o.TargetName, `/\`) || reserved(o.TargetName) ||
		(o.ObservedName != "" && (filepath.Base(o.ObservedName) != o.ObservedName || strings.ContainsAny(o.ObservedName, `/\`) || reserved(o.ObservedName))) {
		return nil, fmt.Errorf("invalid frozen rename names")
	}
	dir := filepath.Dir(filepath.FromSlash(in.Unit.RootPath))
	source := in.Unit.RootPath
	if o.ObservedName != "" {
		source = filepath.ToSlash(filepath.Join(dir, o.ObservedName))
	}
	target := filepath.ToSlash(filepath.Join(dir, o.TargetName))
	return &preparedRename{
		task:     t,
		root:     in.WorksetRoot,
		action:   o.Action,
		recorded: in.Unit.RootPath,
		rebind:   o.RebindTo,
		result:   Result{SourcePath: source, TargetPath: target},
	}, nil
}

type preparedRename struct {
	task         *Task
	root, action string
	// recorded is where the stored paths still point; it differs from the
	// result's source only for a folder that was renamed by hand, and equals the
	// target when such a folder is renamed back.
	recorded string
	// rebind is the work code the stored copy takes on with the new name.
	rebind string
	result Result
}

func (*preparedRename) EncodeTasks() int { return 0 }
func (*preparedRename) EncodeTask(context.Context, int) error {
	return fmt.Errorf("rename has no encode tasks")
}

func (p *preparedRename) Commit(ctx context.Context) (workset.UnitResult, error) {
	if err := ctx.Err(); err != nil {
		return p.Discard(err), err
	}
	if p.action == "blocked" {
		return p.observed(), nil
	}
	if p.task.io.ResolveDirectory == nil || p.task.io.Rename == nil || p.task.io.Relocate == nil {
		return p.failure("COMPONENT_RENAME_FAILED", fmt.Errorf("rename execution is not configured")), nil
	}
	rel, err := filepath.Rel(filepath.FromSlash(p.root), filepath.FromSlash(p.result.SourcePath))
	if err != nil {
		return p.failure("COMPONENT_RENAME_FAILED", err), nil
	}
	source, err := p.task.io.ResolveDirectory(p.root, filepath.ToSlash(rel))
	if err != nil {
		return p.failure("INPUT_CHANGED", err), nil
	}
	if p.action == "noop" {
		// A hand-renamed folder that already has its target name: only the
		// records move to where it is, even if cancellation arrives during reconciliation.
		if p.recorded != p.result.SourcePath {
			if err := p.task.io.Relocate(
				context.WithoutCancel(ctx), p.root, p.recorded, p.result.SourcePath, "",
			); err != nil {
				return p.failure("RENAME_STATE_SYNC_FAILED", err), nil
			}
		}
		return p.observed(), nil
	}
	if err := ctx.Err(); err != nil {
		return p.Discard(err), err
	}
	// The no-replace rename is the guard against an occupied target; a listing
	// of the whole parent per unit would make a flat library quadratic.
	if err := p.task.io.Rename(source, filepath.FromSlash(p.result.TargetPath)); err != nil {
		if errors.Is(err, fs.ErrExist) {
			err = fmt.Errorf("target already exists: %w", err)
		}
		return p.failure("COMPONENT_RENAME_FAILED", err), nil
	}
	p.result.Renamed = true
	// Cancellation stops the next unit; this disk/database boundary must finish.
	if err := p.task.io.Relocate(
		context.WithoutCancel(ctx),
		p.root,
		p.recorded,
		p.result.TargetPath,
		p.rebind,
	); err != nil {
		rollback := p.task.io.Rename(filepath.FromSlash(p.result.TargetPath), source)
		code := "RENAME_STATE_SYNC_FAILED"
		if rollback == nil {
			p.result.Renamed, p.result.RolledBack = false, true
		} else {
			code = "RENAME_ROLLBACK_FAILED"
			err = fmt.Errorf("%w; rollback: %w", err, rollback)
		}
		result := p.failure(code, err)
		result.InventoryError = err.Error()
		return result, nil
	}
	return p.observed(), nil
}

func (p *preparedRename) observed() workset.UnitResult {
	raw, _ := json.Marshal(p.result)
	count := 0
	result := workset.UnitResult{TaskResult: raw, CompletedOperations: &count}
	if p.result.Renamed {
		count = 1
		result.Committed = []string{p.result.TargetPath}
		result.Removed = []string{p.result.SourcePath}
	}
	return result
}
func (p *preparedRename) failure(code string, err error) workset.UnitResult {
	result := p.observed()
	result.Stage, result.ErrorCode, result.ErrorMessage = "commit", code, err.Error()
	return result
}
func (p *preparedRename) Discard(cause error) workset.UnitResult {
	result := p.observed()
	if cause != nil {
		result.ErrorMessage = cause.Error()
		result.Canceled = errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded)
	}
	return result
}
