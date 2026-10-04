package workset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// BatchExecutor owns work that needs a complete frozen worklist instead of an encode pool.
type BatchExecutor func(context.Context, BatchExecutionInput) error
type BatchExecutionInput struct {
	ExecutionID string
	LibraryRoot string
	Units       []ExecutionUnit
	Options     json.RawMessage
	Progress    func(int, UnitResult) error
}

func (d *dispatcher) executeBatch(ctx context.Context, ex *PlanExecution, run *executionRun, execute BatchExecutor) {
	report := run.report
	byIndex := map[int]int{}
	for i, unit := range run.req.Units {
		byIndex[unit.Index] = i
	}
	progress := func(index int, result UnitResult) error {
		at, ok := byIndex[index]
		if !ok {
			return fmt.Errorf("batch result does not belong to frozen worklist")
		}
		entry := &report[at]
		applyUnitResult(entry, result)
		entry.InventorySynced = true
		return d.persistUnitResult(ex.ExecutionID, entry, ExecutionUnit{})
	}
	err := execute(
		ctx,
		BatchExecutionInput{
			ExecutionID: ex.ExecutionID,
			LibraryRoot: run.rootPath,
			Units:       run.req.Units,
			Options:     run.req.Options,
			Progress:    progress,
		},
	)
	status, code, message := ExecStatusSucceeded, "", ""
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		status, code, message = ExecStatusCanceled, "CANCELED", "metadata execution canceled"
	} else if err != nil {
		status = ExecStatusFailed
		code, message = taskFailureOf(err)
	}
	for i := range report {
		if report[i].Status == ExecComponentFailed && status == ExecStatusSucceeded {
			status, code, message = ExecStatusFailed, "METADATA_PARTIAL_FAILURE", "one or more works could not be refreshed"
		}
		if report[i].Status == ExecComponentPending && status != ExecStatusSucceeded {
			report[i].Status = ExecComponentCanceled
			if status == ExecStatusFailed {
				report[i].Status = ExecComponentFailed
			}
			report[i].ErrorCode = code
			report[i].ErrorMessage = message
			if e := d.persistUnitResult(ex.ExecutionID, &report[i], ExecutionUnit{}); e != nil {
				status = ExecStatusFailed
				code, message = taskFailureOf(e)
			}
		}
	}
	d.finishExecution(ex, status, code, message)
}
