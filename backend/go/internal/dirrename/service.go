package dirrename

import (
	"context"

	"github.com/onsei/organizer/backend/internal/workset"
)

// Catalog reads the binding and metadata of library directories in one snapshot.
type Catalog interface {
	RenameWorks(context.Context, string, []string) (map[string]Work, error)
}

// Work contains only the metadata available to naming templates.
type Work struct {
	Code   string            `json:"code"`
	Values map[string]string `json:"values"`
}

// Entry describes an immediate parent entry without following symlinks.
type Entry struct {
	Directory bool  `json:"directory"`
	Symlink   bool  `json:"symlink"`
	Modified  int64 `json:"modified"`
}

// Directory is a read-only view of immediate parent entries.
type Directory map[string]Entry

// ExecutionIO supplies directory moves and atomic path-state relocation.
type ExecutionIO struct {
	ResolveDirectory func(root, relative string) (string, error)
	Rename           func(source, target string) error
	Relocate         func(context.Context, string, string, string) error
	PendingCovers    func(context.Context, string, []string) (bool, error)
}

// Task plans and executes frozen work-directory basename changes.
type Task struct {
	catalog       Catalog
	readDirectory func(context.Context, string) (Directory, error)
	io            ExecutionIO
}

// New wires naming reads and optional move, path-relocation and cover-admission dependencies.
func New(
	catalog Catalog,
	readDirectory func(context.Context, string) (Directory, error),
	execution ...ExecutionIO,
) *Task {
	t := &Task{catalog: catalog, readDirectory: readDirectory}
	if len(execution) > 0 {
		t.io = execution[0]
	}
	return t
}

func (*Task) Kind() string                     { return "rename" }
func (*Task) RequiresAudio() bool              { return false }
func (*Task) RequiresExclusiveExecution() bool { return true }

var _ workset.Task = (*Task)(nil)
