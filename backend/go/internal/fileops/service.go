// Package fileops owns direct file management inside a library member
// (rename, move, soft delete): the scope and path rules it refuses, the
// per-item result contract, and the write-then-refresh behaviour. Admission
// against the other paths that touch the same trees is package admission
// (ADR 0002 §1, §2).
package fileops

import (
	"context"
	"errors"
	"strings"

	"github.com/onsei/organizer/backend/internal/admission"
	"github.com/onsei/organizer/backend/internal/librarypath"
)

// MemberScan refreshes one directory subtree in the stored inventory. It is
// the same disk → inventory seam the planning sessions use, handed in so the
// service does not depend on the scanner.
type MemberScan func(ctx context.Context, folderPath, rootPath string) error

// RecoverFile moves an item into the recovery directory beside its work root.
type RecoverFile func(ctx context.Context, workRoot, source string) (string, error)

// RenameFile moves an item without replacing an existing destination.
type RenameFile func(source, target string) error

// ResolvedWithinRoot checks existing paths after host filesystem indirection.
// The consumer needs only containment, not a general path-resolution service.
type ResolvedWithinRoot func(root, candidate string) (bool, error)

// Service applies direct file-management requests inside a library member.
type Service struct {
	gate        *admission.Gate
	scan        MemberScan
	recoverFile RecoverFile
	renameFile  RenameFile
	withinRoot  ResolvedWithinRoot
}

// NewService creates the file-management service. gate is the process-wide
// admission control; scan refreshes the affected inventory after a write (nil
// means the writes are reported without a refresh, which the tests use).
// recoverFile and renameFile provide moves; withinRoot resolves host filesystem
// indirection without exposing a general filesystem interface.
func NewService(
	gate *admission.Gate,
	scan MemberScan,
	recoverFile RecoverFile,
	renameFile RenameFile,
	withinRoot ResolvedWithinRoot,
) *Service {
	return &Service{gate: gate, scan: scan, recoverFile: recoverFile, renameFile: renameFile, withinRoot: withinRoot}
}

// Apply runs one request: admission first, then the items in order, then the
// refresh of everything the writes touched.
//
// The whole request — including its refresh — runs inside one direct-file-
// management slot, so the refresh is part of the same protected operation and
// never re-enters admission against itself (ADR 0002 §2).
//
// Execution stops at the first failure: the remaining items are reported as
// not attempted, and nothing already done is undone. A failed refresh is
// reported as a refresh failure, never as a failed write (ADR 0002 §1; ADR 0001 §4).
func (s *Service) Apply(ctx context.Context, req Request) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch req.Operation {
	case OpRename, OpMove, OpSoftDelete:
	default:
		return nil, &PathError{Code: CodeOperationDenied, Message: "unsupported operation " + req.Operation}
	}
	if s.gate == nil {
		return nil, errors.New("file operations are not wired to admission control")
	}
	if s.recoverFile == nil || s.renameFile == nil || s.withinRoot == nil {
		return nil, errors.New("file moves are not wired")
	}
	release, err := s.gate.BeginManual()
	if err != nil {
		return nil, err
	}
	defer release()

	m, err := openMember(req.LibraryRoot, req.MemberPath, s.withinRoot)
	if err != nil {
		return nil, err
	}
	if len(req.Items) == 0 {
		return nil, &PathError{CodePathInvalid, "the request has no items"}
	}
	// A rename and a move each act on one item; only soft delete is a batch
	// (ADR 0002 §1). A request that asks for several renames is refused as a
	// whole — before the member is touched — rather than applied item by item.
	if req.Operation != OpSoftDelete && len(req.Items) != 1 {
		return nil, &PathError{
			CodePathInvalid,
			"a " + req.Operation + " takes exactly one item; only a soft delete is a batch",
		}
	}

	worklist := planItems(req.Items)
	result := &Result{
		Operation:  req.Operation,
		MemberPath: m.rel,
		Items:      make([]ItemResult, 0, len(worklist)),
	}

	stopped := false
	for _, planned := range worklist {
		switch {
		case planned.skip != nil:
			message := "covered by the selected parent directory"
			if *planned.skip == CodeDuplicateItem {
				message = "named more than once"
			}
			result.Items = append(result.Items, ItemResult{
				Source: planned.item.Source, Status: StatusSkipped, Code: *planned.skip,
				Message: message,
			})
		case stopped:
			result.Items = append(result.Items, ItemResult{
				Source: planned.item.Source, Status: StatusNotAttempted,
				Message: "not attempted: an earlier item failed",
			})
		default:
			itemResult := s.applyOperation(ctx, m, req.Operation, planned.item)
			result.Items = append(result.Items, itemResult)
			if itemResult.Status == StatusFailed {
				stopped = true
			}
		}
	}

	for _, item := range result.Items {
		switch item.Status {
		case StatusOK:
			result.Succeeded++
		case StatusFailed:
			result.Failed++
		default:
			result.Untouched++
		}
	}
	result.Refresh = s.refreshAfter(ctx, m)
	return result, nil
}

// planned pairs one requested item with its decision: applied, or skipped
// because a selected ancestor covers it.
type planned struct {
	item Item
	skip *string
}

// planItems keeps the request order and drops the items a selected ancestor
// already covers — a directory and its own child are one operation, and
// deleting the parent first would otherwise leave the child "missing" — as
// well as an item named twice, which is the same operation asked for twice
// (ADR 0002 §1).
func planItems(items []Item) []planned {
	cleaned := make([]string, len(items))
	for i, item := range items {
		if rel, ok := librarypath.RelPath(item.Source); ok {
			cleaned[i] = rel
		} else {
			cleaned[i] = item.Source
		}
	}
	out := make([]planned, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for i, item := range items {
		source := cleaned[i]
		var reason *string
		if _, duplicate := seen[source]; duplicate {
			code := CodeDuplicateItem
			reason = &code
			out = append(out, planned{item: item, skip: reason})
			continue
		}
		for j, ancestor := range cleaned {
			if i == j {
				continue
			}
			if isDescendant(source, ancestor) {
				code := CodeCoveredByParent
				reason = &code
				break
			}
		}
		if reason == nil {
			seen[source] = struct{}{}
		}
		out = append(out, planned{item: item, skip: reason})
	}
	return out
}

// isDescendant reports whether candidate is strictly below ancestor.
func isDescendant(candidate, ancestor string) bool {
	if ancestor == "" || candidate == ancestor {
		return false
	}
	return strings.HasPrefix(candidate, strings.TrimSuffix(ancestor, "/")+"/")
}

// refreshAfter re-reads the member inventory; recovery files are excluded from
// scanning. The refresh keeps the same admission as the writes.
func (s *Service) refreshAfter(ctx context.Context, m *member) RefreshState {
	if s.scan == nil {
		return RefreshState{OK: true}
	}
	if err := s.scan(ctx, m.nativeAbs, m.nativeRoot); err != nil {
		return RefreshState{
			OK:      false,
			Code:    "REFRESH_FAILED",
			Message: "files were modified, but refreshing the inventory failed: " + err.Error(),
		}
	}
	return RefreshState{OK: true}
}

// RecoveryDir is the work-parent-level directory that receives recycled
// media. It is not a member and never a browsing scope.
const RecoveryDir = librarypath.RecoveryDirName

// ResolveMember validates a library-relative member path for reading (no
// admission, no write): it must be a descendant directory of the library
// root with real directories along every component. Reading a member tree and writing
// inside one resolve the member through this same function, so both refuse the
// same paths.
func ResolveMember(rootPath, memberRel string, withinRoot ResolvedWithinRoot) (string, error) {
	m, err := openMember(rootPath, memberRel, withinRoot)
	if err != nil {
		return "", err
	}
	return m.nativeAbs, nil
}

// ResolveMemberFile validates one member-relative item path and resolves it,
// under exactly the checks an operation's own target goes through (ADR 0002
// §1): every component must exist, none may be a symlink, and the resolved
// form must stay inside the member. Read-only consumers (ADR 0016 §3) share
// the seam so a link can never be read through either.
func ResolveMemberFile(rootPath, memberRel, rel string, withinRoot ResolvedWithinRoot) (string, error) {
	m, err := openMember(rootPath, memberRel, withinRoot)
	if err != nil {
		return "", err
	}
	return m.resolve(rel)
}
