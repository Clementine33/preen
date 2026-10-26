package coverdelivery

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/Clementine33/preen/backend/internal/admission"
)

// ScheduleRefresh registers a batch after metadata finishes enqueueing covers.
// Batches without cover demands do not register. This follow-up belongs to
// the process lifecycle; persistent cover delivery recovery remains independent.
// shortcut: pending follow-ups are process-local; persist them if restart
// continuity becomes a requirement.
func (s *Service) ScheduleRefresh(executionID, root string) {
	if s.deps.RefreshSelected == nil {
		return
	}
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.refreshes[executionID] = root
}

// RefreshNext attempts one ready execution's inventory reconciliation. Demand
// states define this execution's boundary.
// Admission waits for metadata's persisted terminal state and other file work.
// A scan failure is independent of metadata and is not automatically retried.
func (s *Service) RefreshNext(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !s.refreshMu.TryLock() {
		return false, nil
	}
	defer s.refreshMu.Unlock()
	for id, root := range s.refreshes {
		report, err := s.deps.Store.CoverReport(ctx, id)
		if err != nil {
			return false, err
		}
		if report.Counts["pending"]+report.Counts["running"] > 0 {
			continue
		}
		folders := []string{}
		for _, item := range report.Items {
			if item.ErrorCode == "INVENTORY_REFRESH_FAILED" {
				folders = append(folders, filepath.ToSlash(filepath.Join(root, item.Target.RelPath)))
			}
		}
		slices.Sort(folders)
		folders = slices.Compact(folders)
		if len(folders) == 0 {
			delete(s.refreshes, id)
			return true, nil
		}
		release, err := s.deps.Acquire()
		if admission.IsBusy(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		defer release()
		delete(s.refreshes, id)
		if err := s.deps.RefreshSelected(ctx, root, folders); err != nil {
			return true, fmt.Errorf("metadata execution %s selected inventory refresh: %w", id, err)
		}
		return true, nil
	}
	return false, nil
}
