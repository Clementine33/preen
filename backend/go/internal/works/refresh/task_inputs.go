package refresh

import (
	"context"
	"errors"
	"io/fs"

	"github.com/onsei/organizer/backend/internal/workset"
)

// RefreshGenerationInputs leaves directory checks to planning, without walking
// or merging the members' contents.
func (*Task) RefreshGenerationInputs(
	ctx context.Context, _ []string, _ string, onRoot func(), _ func() error,
) error {
	if onRoot != nil {
		onRoot()
	}
	return ctx.Err()
}

// NewExecutionSession checks only the actual frozen worklist. Metadata still
// runs through its batch executor; no unit or output revalidation follows it.
func (t *Task) NewExecutionSession(
	ctx context.Context,
	in workset.ExecutionSessionInput,
) (workset.UnitPreparer, error) {
	seen := map[string]bool{}
	for _, unit := range in.Units {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if seen[unit.RootPath] {
			continue
		}
		seen[unit.RootPath] = true
		if err := t.checkDirectory(unit.RootPath); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			code := "SCAN_FAILED"
			if errors.Is(err, fs.ErrNotExist) {
				code = "INPUT_CHANGED"
			}
			return nil, workset.NewError(
				workset.ErrKindConflict,
				code,
				"cannot check frozen work directory: "+unit.RootPath,
				err,
			)
		}
	}
	return t, ctx.Err()
}
