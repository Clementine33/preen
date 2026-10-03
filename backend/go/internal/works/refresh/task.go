// Package metadatatask supplies workset planning and batched metadata refresh.
package refresh

import (
	"context"

	"github.com/onsei/organizer/backend/internal/works/coverdelivery"
	"github.com/onsei/organizer/backend/internal/workset"
)

const Kind = "metadata_refresh"

type TaskCatalog interface {
	CoverTargets(context.Context, string, []string) (map[string]coverdelivery.Target, error)
}
type Task struct {
	catalog  TaskCatalog
	identity func(string) (string, error)
}

func NewTask(c TaskCatalog, identity func(string) (string, error)) *Task {
	return &Task{catalog: c, identity: identity}
}
func (*Task) Kind() string        { return Kind }
func (*Task) RequiresAudio() bool { return false }

var _ workset.Task = (*Task)(nil)
