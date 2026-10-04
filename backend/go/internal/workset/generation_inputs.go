package workset

import "context"

// GenerationInputRefresher optionally batches a task's selected inventory
// refresh. The fallback preserves the original serial pipeline for other tasks.
type GenerationInputRefresher interface {
	RefreshGenerationInputs(context.Context, []string, string, func(), func() error) error
}

func (s *serviceImpl) refreshGenerationInputs(
	ctx context.Context,
	kind string,
	folders []string,
	root string,
	onRoot func(),
) error {
	fallback := func() error { return s.refreshRoots(ctx, folders, root, onRoot) }
	task, err := s.requireTask(kind)
	if err != nil {
		return err
	}
	if refresher, ok := task.(GenerationInputRefresher); ok {
		return refresher.RefreshGenerationInputs(ctx, folders, root, onRoot, fallback)
	}
	return fallback()
}
