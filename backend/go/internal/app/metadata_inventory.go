package app

import (
	"context"
	"path/filepath"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

// Metadata's final reconciliation shares bounded folder walks, one staging
// writer and one selected-range merge. It runs under coverdelivery's admission.
func metadataInventoryRefresh(
	pipeline *inventory.Pipeline,
) func(context.Context, string, []string) error {
	return func(ctx context.Context, root string, folders []string) error {
		for _, folder := range folders {
			rel, err := filepath.Rel(filepath.FromSlash(root), filepath.FromSlash(folder))
			if err != nil {
				return err
			}
			if _, err := filesystem.ResolveMember(root, filepath.ToSlash(rel)); err != nil {
				return err
			}
		}
		_, err := pipeline.ScanFoldersCtx(ctx, folders, root, inventory.DefaultScanWorkers)
		return err
	}
}
