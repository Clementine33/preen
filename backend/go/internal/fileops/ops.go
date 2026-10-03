package fileops

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/onsei/organizer/backend/internal/pathnorm"
)

// Operation names accepted by Apply. They are the first version's whole
// vocabulary: no overwrite, no cross-member move, no directory creation, no
// copy-then-delete across filesystems (ADR 0002 §1).
const (
	OpRename     = "rename"
	OpMove       = "move"
	OpSoftDelete = "soft_delete"
)

// Item statuses of one applied request.
const (
	StatusOK           = "ok"
	StatusFailed       = "failed"
	StatusSkipped      = "skipped"
	StatusNotAttempted = "not_attempted"
)

// Coverage codes that explain a skipped item.
const (
	CodeCoveredByParent = "COVERED_BY_PARENT"
	CodeDuplicateItem   = "DUPLICATE_ITEM"
)

// Request is one file-management request: an operation applied to items inside
// one member directory.
type Request struct {
	LibraryRoot string // the owning library's root path, as stored
	MemberPath  string // library-relative member directory
	Operation   string // rename | move | soft_delete
	Items       []Item // applied in order
}

// Item is one target of the request. Source is member-relative; Name is the
// rename's new name (a name, never a path) and TargetDir the move's
// member-relative destination directory.
type Item struct {
	Source    string `json:"source"`
	Name      string `json:"name,omitempty"`
	TargetDir string `json:"target_dir,omitempty"`
}

// ItemResult is what happened to one item. Target is where the item ended up
// (member-relative for rename and move, library-relative for a recycled item),
// so the caller can report and copy the actual location.
type ItemResult struct {
	Source  string `json:"source"`
	Status  string `json:"status"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	// Target is where a rename or move put the item, member-relative.
	Target string `json:"target,omitempty"`
	// RecoveredPath is where a soft delete put the item, relative to the
	// library root. It is the path the user restores from with their own file
	// manager, so it is reported exactly as it is on disk.
	RecoveredPath string `json:"recovered_path,omitempty"`
}

// RefreshState reports the inventory refresh that follows a write. A failed
// refresh never hides the writes that already happened: the two facts are
// reported separately (ADR 0001 §4).
type RefreshState struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Result is the whole outcome of one request: per-item facts, their counts and
// the refresh state.
type Result struct {
	Operation  string       `json:"operation"`
	MemberPath string       `json:"member_path"`
	Items      []ItemResult `json:"items"`
	Succeeded  int          `json:"succeeded"`
	Failed     int          `json:"failed"`
	Untouched  int          `json:"untouched"`
	Refresh    RefreshState `json:"refresh"`
}

// applyOperation performs one validated operation and returns the item's
// result. It never overwrites: every destination is claimed with a no-replace
// rename, and a name that appeared since the check is a refusal, not a
// clobber.
func (s *Service) applyOperation(ctx context.Context, m *member, op string, item Item) ItemResult {
	switch op {
	case OpRename:
		return s.renameItem(m, item)
	case OpMove:
		return s.moveItem(m, item)
	case OpSoftDelete:
		return s.softDeleteItem(ctx, m, item)
	default:
		return failed(item.Source, CodeOperationDenied, "unsupported operation "+op)
	}
}

// renameItem changes one item's name inside its own directory.
func (s *Service) renameItem(m *member, item Item) ItemResult {
	name, err := targetName(item.Name)
	if err != nil {
		return fromError(item.Source, err)
	}
	source, err := m.resolve(item.Source)
	if err != nil {
		return fromError(item.Source, err)
	}
	target := filepath.Join(filepath.Dir(source), filepath.FromSlash(name))
	if err := claimTarget(m, source, target); err != nil {
		return fromError(item.Source, err)
	}
	if err := s.renameFile(source, target); err != nil {
		return fromError(item.Source, err)
	}
	return ItemResult{Source: item.Source, Status: StatusOK, Target: memberRelative(m, target)}
}

// moveItem moves one item into an existing directory of the same member. The
// destination directory is resolved first, so a move whose target directory
// vanished or is not a directory is refused before anything moves.
func (s *Service) moveItem(m *member, item Item) ItemResult {
	if strings.TrimSpace(item.TargetDir) == "" {
		return failed(item.Source, CodeInvalidTarget, "a move needs a destination directory")
	}
	source, err := m.resolve(item.Source)
	if err != nil {
		return fromError(item.Source, err)
	}
	dir, err := m.resolveTargetDir(item.TargetDir)
	if err != nil {
		return fromError(item.Source, err)
	}
	target := filepath.Join(dir, filepath.Base(source))
	if samePath(source, target) {
		return failed(item.Source, CodeInvalidTarget, "the item is already in that directory")
	}
	if err := refuseIntoSelf(source, dir); err != nil {
		return fromError(item.Source, err)
	}
	if err := claimTarget(m, source, target); err != nil {
		return fromError(item.Source, err)
	}
	if err := s.renameFile(source, target); err != nil {
		return fromError(item.Source, err)
	}
	return ItemResult{Source: item.Source, Status: StatusOK, Target: memberRelative(m, target)}
}

// softDeleteItem moves one item into the recovery directory beside its work,
// keeping the work name and its internal path, and reports where it went.
// An existing recovery file is never overwritten: a collision gets the first
// free "<stem>.N<ext>" name.
func (s *Service) softDeleteItem(ctx context.Context, m *member, item Item) ItemResult {
	source, err := m.resolve(item.Source)
	if err != nil {
		return fromError(item.Source, err)
	}
	target, err := s.recoverFile(ctx, m.nativeAbs, source)
	if err != nil {
		return fromError(item.Source, err)
	}
	relToLibrary, relErr := filepath.Rel(m.nativeRoot, target)
	if relErr != nil {
		relToLibrary = target
	}
	return ItemResult{
		Source:        item.Source,
		Status:        StatusOK,
		RecoveredPath: pathnorm.NormalizeToPOSIX(relToLibrary),
	}
}

// claimTarget refuses a destination that exists or that would leave the
// member, immediately before the rename. The no-replace rename below closes
// the window that remains; this check is what gives a stable, explainable
// refusal instead of a raw filesystem error.
func claimTarget(m *member, source, target string) error {
	if !insideMember(m.nativeAbs, target) {
		return &PathError{CodeOutsideMember, "the destination is outside the member"}
	}
	if _, err := os.Lstat(target); err == nil {
		return &PathError{CodeTargetExists, "the destination already exists"}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return &PathError{errCode(err), err.Error()}
	}
	return nil
}

// refuseIntoSelf refuses moving a directory into itself or into one of its own
// descendants: the move would detach the subtree from where it was read.
func refuseIntoSelf(source, dir string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return &PathError{errCode(err), err.Error()}
	}
	if !info.IsDir() {
		return nil
	}
	if insideMember(source, dir) {
		return &PathError{CodeMoveIntoSelf, "a directory cannot be moved into itself or its own subdirectory"}
	}
	return nil
}

// memberRelative renders an absolute path as member-relative POSIX form.
func memberRelative(m *member, native string) string {
	rel, err := filepath.Rel(m.nativeAbs, native)
	if err != nil {
		return pathnorm.NormalizeToPOSIX(native)
	}
	return pathnorm.NormalizeToPOSIX(rel)
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func failed(source, code, message string) ItemResult {
	return ItemResult{Source: source, Status: StatusFailed, Code: code, Message: message}
}

// fromError turns an operation error into a failed item result.
func fromError(source string, err error) ItemResult {
	if itemErr, ok := errors.AsType[*PathError](err); ok {
		return failed(source, itemErr.Code, itemErr.Message)
	}
	if linkErr, ok := errors.AsType[*os.LinkError](err); ok {
		return failed(source, errCode(linkErr.Err), linkErr.Err.Error())
	}
	if pathErr, ok := errors.AsType[*os.PathError](err); ok {
		return failed(source, errCode(pathErr.Err), pathErr.Err.Error())
	}
	return failed(source, errCode(err), err.Error())
}
