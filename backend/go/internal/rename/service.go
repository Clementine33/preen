package rename

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

// Entry describes an immediate library entry without following symlinks.
type Entry struct {
	Directory bool  `json:"directory"`
	Symlink   bool  `json:"symlink"`
	Modified  int64 `json:"modified"`
}

// Directory is a read-only view of immediate library entries.
type Directory map[string]Entry

// Task plans top-level directory names without writing to disk.
type Task struct {
	catalog       Catalog
	readDirectory func(context.Context, string) (Directory, error)
}

// New builds a rename task from its read-only dependencies.
func New(catalog Catalog, readDirectory func(context.Context, string) (Directory, error)) *Task {
	return &Task{catalog: catalog, readDirectory: readDirectory}
}

func (*Task) Kind() string        { return "rename" }
func (*Task) RequiresAudio() bool { return false }

var _ workset.Task = (*Task)(nil)
