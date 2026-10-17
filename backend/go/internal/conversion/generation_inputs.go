package conversion

import (
	"context"
	"time"
)

// SelectedScan refreshes explicit participating scopes with one writer/merge;
// its callback checks cancellation during the refresh and while waiting for it.
type SelectedScan func(context.Context, []string, string, func()) error

// Keep simultaneous sessions from multiplying the scan width on shared disks.
func serializeSelectedScan(scan SelectedScan) SelectedScan {
	if scan == nil {
		return nil
	}
	slot := make(chan struct{}, 1)
	return func(ctx context.Context, folders []string, root string, onRoot func()) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case slot <- struct{}{}:
				defer func() { <-slot }()
				if err := ctx.Err(); err != nil {
					return err
				}
				return scan(ctx, folders, root, onRoot)
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				// Workset cancellation is observed by this callback, even while waiting.
				if onRoot != nil {
					onRoot()
				}
			}
		}
	}
}

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
