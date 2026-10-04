package coverdelivery

import (
	"context"
	"fmt"
	"slices"

	"github.com/onsei/organizer/backend/internal/admission"
)

type selectedRefresh struct {
	root    string
	folders []string
}

// ScheduleRefresh seals the selected scope when metadata finishes enqueueing.
// Empty executions never schedule filesystem work. This follow-up belongs to
// the process lifecycle; persistent cover delivery recovery remains independent.
// ponytail: pending follow-ups are process-local; persist them if restart
// continuity becomes a requirement.
func (s *Service) ScheduleRefresh(executionID, root string, folders []string) {
	if s.deps.RefreshSelected == nil || len(folders) == 0 {
		return
	}
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.refreshes[executionID] = selectedRefresh{root: root, folders: slices.Clone(folders)}
}

// RefreshNext attempts one ready execution's inventory reconciliation. Demand
// states, rather than shared download states, define this execution's boundary.
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
	for id, refresh := range s.refreshes {
		report, err := s.deps.Store.CoverReport(ctx, id)
		if err != nil {
			return false, err
		}
		if report.Counts["pending"]+report.Counts["running"] > 0 {
			continue
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
		if err := s.deps.RefreshSelected(ctx, refresh.root, refresh.folders); err != nil {
			return true, fmt.Errorf("metadata execution %s selected inventory refresh: %w", id, err)
		}
		return true, nil
	}
	return false, nil
}
