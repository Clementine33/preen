package refresh

import "github.com/Clementine33/preen/backend/internal/workset"

// The worker's admission gate delays this follow-up until execution is terminal;
// coverdelivery then waits for only this execution's delivery demands.
func (e Executor) scheduleInventoryRefresh(in workset.BatchExecutionInput) {
	if e.Covers == nil {
		return
	}
	folders := make([]string, 0, len(in.Units))
	for _, unit := range in.Units {
		folders = append(folders, unit.RootPath)
	}
	e.Covers.ScheduleRefresh(in.ExecutionID, in.LibraryRoot, folders)
}
