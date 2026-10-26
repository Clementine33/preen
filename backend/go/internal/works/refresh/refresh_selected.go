package refresh

import "github.com/Clementine33/preen/backend/internal/workset"

// The worker's admission gate delays this follow-up until execution is terminal;
// coverdelivery then waits for only this execution's delivery demands.
func (e Executor) scheduleInventoryRefresh(in workset.BatchExecutionInput) {
	if e.Covers == nil {
		return
	}
	e.Covers.ScheduleRefresh(in.ExecutionID, in.LibraryRoot)
}
