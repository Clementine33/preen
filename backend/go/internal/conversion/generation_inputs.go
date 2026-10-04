package conversion

import "context"

// SelectedScan refreshes explicit participating scopes with one writer/merge.
type SelectedScan func(context.Context, []string, string, func()) error

func (t *Task) RefreshGenerationInputs(
	ctx context.Context,
	folders []string,
	root string,
	onRoot func(),
	fallback func() error,
) error {
	if t.selectedScan == nil {
		return fallback()
	}
	return t.selectedScan(ctx, folders, root, onRoot)
}
