package workset

import (
	"context"
	"log/slog"
	"strings"
)

// Failure logging: the database rows that carry an execution's or generation's
// failure are replaced by the next run, so every failure is also written to the
// process log (ADR 0006 keeps the rows authoritative; the log is the record that
// outlives them). Cancellation and shutdown are outcomes, not failures, and are
// never logged here.

func executionAttrs(ex *PlanExecution) []any {
	return []any{
		"execution_id", ex.ExecutionID, "plan_id", ex.PlanID,
		"workset_id", ex.WorksetID, "operation_type", ex.OperationType,
	}
}

// causeAttrs is the `err` field of a failure: a workset error contributes its
// cause, any other error itself, and nothing when the message already carries
// the text (task errors fold their cause into the message).
func causeAttrs(message string, err error) []any {
	if err == nil {
		return nil
	}
	if werr, ok := AsError(err); ok {
		err = werr.Cause
		if err == nil {
			return nil
		}
	}
	if strings.Contains(message, err.Error()) {
		return nil
	}
	return []any{"err", err}
}

// logExecutionFailed records a failed session before its terminal write, so the
// reason survives a failing write.
func logExecutionFailed(ex *PlanExecution, code, message string, cause error, extra []any) {
	attrs := append(executionAttrs(ex), "error_code", code, "error_message", message)
	attrs = append(attrs, extra...)
	slog.Error("execution failed", append(attrs, causeAttrs(message, cause)...)...)
}

// logComponentFailed records one component that failed on its own. Components
// a batch rewrites to the session's outcome are summarized by the session line.
func logComponentFailed(ex *PlanExecution, entry *ExecutionComponentView) {
	slog.Warn("execution component failed", append(executionAttrs(ex),
		"component_index", entry.ComponentIndex, "component_id", entry.ComponentID,
		"root_path", entry.RootPath, "stage", entry.Stage,
		"error_code", entry.ErrorCode, "error_message", entry.ErrorMessage)...)
}

// warnUnrequestedCancel flags an error read as cancellation while nothing asked
// to cancel: classification looks at the error's shape, so a deadline error
// would otherwise turn a real failure into a silent cancel. It must run where
// the error is classified, before the pool's teardown cancels the context.
func warnUnrequestedCancel(ctx context.Context, ex *PlanExecution, err error) {
	if err == nil || ctx.Err() != nil || !sessionCanceled(ctx, err) {
		return
	}
	logUnrequestedCancel(ex, err)
}

func logUnrequestedCancel(ex *PlanExecution, err error) {
	slog.Warn("execution canceled without a cancel request", append(executionAttrs(ex), "err", err)...)
}

func logGenerationFailed(gen *PlanGeneration, code, message string, cause error) {
	attrs := []any{
		"generation_id", gen.GenerationID, "workset_id", gen.WorksetID, "operation_type", gen.OperationType,
		"error_code", code, "error_message", message,
	}
	slog.Error("generation failed", append(attrs, causeAttrs(message, cause)...)...)
}
