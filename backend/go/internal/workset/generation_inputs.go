package workset

import "context"

// GenerationInputRefresher lets a task batch inventory refresh or leave its
// directory-only checks to planning. The fallback refreshes members serially.
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
