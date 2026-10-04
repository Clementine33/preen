package workset

import (
	"context"
)

// FolderScan refreshes one folder subtree's stored inventory from disk. It is
// the fallback seam for inventory-based tasks. Directory-only tasks replace
// generation refresh and execution prechecks through their session capabilities.
//
// rootPath is the owning library's root: entries are keyed by it, so a scoped
// scan must not rewrite that key with the folder it walked.
type FolderScan func(ctx context.Context, folderPath, rootPath string) error

// refreshRoots is the serial inventory refresh fallback and conversion's
// best-effort refresh after input drift. onRoot observes cooperative cancellation
// between folders; tasks using directory-only inputs do not call this flow.
func (s *serviceImpl) refreshRoots(
	ctx context.Context,
	folders []string,
	rootPath string,
	onRoot func(),
) error {
	if s.scanFolder == nil {
		return nil
	}
	for _, folder := range folders {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.scanFolder(ctx, folder, rootPath); err != nil {
			return err
		}
		if onRoot != nil {
			onRoot()
		}
	}
	return ctx.Err()
}

// participatingRoots lists the folders a planning session will cover: the
// members that take part in the frozen draft, in workset order.
func (s *serviceImpl) participatingRoots(
	operationType string,
	draft []byte,
	members []*WorksetMember,
) ([]string, error) {
	task, err := s.requireTask(operationType)
	if err != nil {
		return nil, err
	}
	facts, err := task.RevisionMembers(RevisionFacts{DraftSnapshot: draft, Members: members})
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(facts))
	for _, f := range facts {
		if !f.Excluded {
			roots = append(roots, f.FolderPath)
		}
	}
	return roots, nil
}

// unitRoots lists the distinct roots a frozen worklist touches, in order.
func unitRoots(units []ExecutionUnit) []string {
	seen := make(map[string]bool, len(units))
	roots := make([]string, 0, len(units))
	for _, u := range units {
		if u.RootPath == "" || seen[u.RootPath] {
			continue
		}
		seen[u.RootPath] = true
		roots = append(roots, u.RootPath)
	}
	return roots
}
