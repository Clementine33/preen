package workset

import (
	"context"
	"encoding/json"
)

// UnitPreparer prepares ordered units; the coordinator remains its sole caller.
type UnitPreparer interface {
	PrepareUnit(context.Context, UnitRunInput) (PreparedUnit, error)
}

// ExecutionSessionFactory optionally replaces the inventory-refresh execution
// gate and fixes task dependencies before unit preparation or batch execution.
// Tasks without this capability retain the existing execution flow.
type ExecutionSessionFactory interface {
	NewExecutionSession(context.Context, ExecutionSessionInput) (UnitPreparer, error)
}

type ExecutionSessionInput struct {
	WorksetRoot string
	Units       []ExecutionUnit
	Outcomes    map[int]json.RawMessage
	Options     json.RawMessage
	Detail      *PlanDetail
	// Refresh is the existing serial member refresh, used only after drift.
	Refresh func(context.Context, []string) error
}

func (d *dispatcher) executionPreparer(
	ctx context.Context,
	ex *PlanExecution,
	run *executionRun,
	task Task,
) (UnitPreparer, error) {
	if factory, ok := task.(ExecutionSessionFactory); ok {
		return factory.NewExecutionSession(ctx, ExecutionSessionInput{
			WorksetRoot: run.rootPath, Units: run.req.Units, Outcomes: run.outcomes,
			Options: run.req.Options, Detail: run.plan,
			Refresh: func(ctx context.Context, roots []string) error {
				return d.svc.refreshRoots(ctx, roots, run.rootPath, nil)
			},
		})
	}
	if code, message, ok := d.verifyInputs(ctx, ex, run); !ok {
		return nil, NewError(ErrKindInternal, code, message, nil)
	}
	return task, nil
}
