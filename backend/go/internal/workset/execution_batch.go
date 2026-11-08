package workset

import (
	"context"
	"encoding/json"
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
	// ProgressBatch persists all results atomically before publishing them to the report.
	ProgressBatch func(map[int]UnitResult) error
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
		return d.persistUnitResult(ex, entry, ExecutionUnit{})
	}
	err := execute(
		ctx,
		BatchExecutionInput{
			ExecutionID: ex.ExecutionID,
			LibraryRoot: run.rootPath,
			Units:       run.req.Units,
			Options:     run.req.Options,
			Progress:    progress,
			ProgressBatch: func(results map[int]UnitResult) error {
				return d.persistBatchResults(ctx, ex.ExecutionID, report, byIndex, results)
			},
		},
	)
	status, code, message := ExecStatusSucceeded, "", ""
	if sessionCanceled(ctx, err) || (err == nil && ctx.Err() != nil) {
		warnUnrequestedCancel(ctx, ex, err)
		status, code, message = cancellationOutcome(ctx)
	} else if err != nil {
		status = ExecStatusFailed
		code, message = taskFailureOf(err)
	}
	rewritten := 0
	for i := range report {
		if report[i].Status == ExecComponentFailed && status != ExecStatusFailed {
			status, code, message = ExecStatusFailed, "METADATA_PARTIAL_FAILURE", "one or more works could not be refreshed"
		}
		if report[i].Status == ExecComponentPending && status != ExecStatusSucceeded {
			report[i].Status = ExecComponentCanceled
			if status == ExecStatusFailed {
				report[i].Status = ExecComponentFailed
			}
			report[i].ErrorCode = code
			report[i].ErrorMessage = message
			rewritten++
			// Never ran: the session line below accounts for it, not one line each.
			if e := d.saveUnitResult(ex.ExecutionID, &report[i], ExecutionUnit{}); e != nil {
				status = ExecStatusFailed
				code, message = taskFailureOf(e)
				err = e
			}
		}
	}
	d.finishExecution(ex, status, code, message, err, "pending_count", rewritten)
}

func (d *dispatcher) persistBatchResults(
	ctx context.Context, executionID string, report []ExecutionComponentView,
	byIndex map[int]int, results map[int]UnitResult,
) error {
	entries := make(map[int]ExecutionComponentView, len(results))
	rows := make([]ExecutionComponentResult, 0, len(results))
	for index, result := range results {
		if err := ctx.Err(); err != nil {
			return err
		}
		at, ok := byIndex[index]
		if !ok {
			return fmt.Errorf("batch result does not belong to frozen worklist")
		}
		entry := report[at]
		applyUnitResult(&entry, result)
		entry.InventorySynced = true
		entries[at] = entry
		rows = append(rows, componentResultRow(&entry))
	}
	if err := retryPersist(ctx, func() error {
		return d.svc.repo.SaveExecutionComponentResults(executionID, rows, executionPositionOf(ExecutionUnit{}))
	}); err != nil {
		return err
	}
	// Failed transactions leave the report pending for executeBatch's teardown.
	for at, entry := range entries {
		report[at] = entry
	}
	return nil
}
