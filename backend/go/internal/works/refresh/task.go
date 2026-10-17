package refresh

import (
	"context"

	"github.com/onsei/organizer/backend/internal/works"
	"github.com/onsei/organizer/backend/internal/works/coverdelivery"
	"github.com/onsei/organizer/backend/internal/workset"
)

const Kind = "metadata_refresh"

type TaskCatalog interface {
	CoverTargets(context.Context, string, []string) (map[string]coverdelivery.Target, error)
	MetadataStates(context.Context, []works.WorkCode) (map[works.WorkCode]map[works.DataGroup]works.SourceState, error)
}
type Task struct {
	catalog        TaskCatalog
	checkDirectory func(string) error
}

// NewTask plans frozen metadata targets using a directory-only check. Bindings
// are read at generation time; later execution retains those frozen targets.
func NewTask(c TaskCatalog, checkDirectory func(string) error) *Task {
	return &Task{catalog: c, checkDirectory: checkDirectory}
}
func (*Task) Kind() string { return Kind }

// LightPlanning declares directory-only generation without inventory scans or probes.
func (*Task) LightPlanning() {}

var _ workset.Task = (*Task)(nil)
