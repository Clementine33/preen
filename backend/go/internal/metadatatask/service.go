// Package metadatatask supplies workset planning and batched metadata refresh.
package metadatatask

import (
	"context"

	"github.com/onsei/organizer/backend/internal/covers"
	"github.com/onsei/organizer/backend/internal/workset"
)

const Kind = "metadata_refresh"

type Catalog interface {
	CoverTargets(context.Context, string, []string) (map[string]covers.Target, error)
}
type Task struct {
	catalog  Catalog
	identity func(string) (string, error)
}

func New(c Catalog, identity func(string) (string, error)) *Task {
	return &Task{catalog: c, identity: identity}
}
func (*Task) Kind() string        { return Kind }
func (*Task) RequiresAudio() bool { return false }

var _ workset.Task = (*Task)(nil)
