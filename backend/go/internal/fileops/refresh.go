package fileops

import (
	"context"

	"github.com/Clementine33/preen/backend/internal/librarypath"
)

// refreshAfter synchronizes successful moves, then scans the member inside the
// held admission slot. Inventory failures never turn a committed disk write into
// a failed item; the scan still runs to reconcile any observed paths.
func (s *Service) refreshAfter(ctx context.Context, m *member, items []ItemResult) RefreshState {
	var syncErr error
	if s.moveInventory != nil {
		for _, item := range items {
			if item.Status != StatusOK {
				continue
			}
			target := ""
			if item.Target != "" {
				target = librarypath.JoinRel(m.abs, item.Target)
			}
			if err := s.moveInventory(
				ctx,
				m.root,
				librarypath.JoinRel(m.abs, item.Source),
				target,
			); err != nil &&
				syncErr == nil {
				syncErr = err
			}
		}
	}
	if s.scan != nil {
		if err := s.scan(ctx, m.nativeAbs, m.nativeRoot); err != nil && syncErr == nil {
			syncErr = err
		}
	}
	if syncErr != nil {
		return RefreshState{
			OK:      false,
			Code:    "REFRESH_FAILED",
			Message: "files were modified, but refreshing the inventory failed: " + syncErr.Error(),
		}
	}
	return RefreshState{OK: true}
}
