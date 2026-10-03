package filesystem

import "context"

// RecoverWithRename exposes the move boundary for recovery failure tests.
func RecoverWithRename(
	ctx context.Context, workRoot, source string, rename func(string, string) error,
) (string, error) {
	return recoverFile(ctx, workRoot, source, rename)
}
