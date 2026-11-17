package filesystem

import (
	"context"
	"io/fs"
)

// RecoverWithRename exposes the move boundary for recovery failure tests.
func RecoverWithRename(
	ctx context.Context, workRoot, source string, rename func(string, string) error,
) (string, error) {
	return recoverFile(ctx, workRoot, source, rename)
}

var ProbeConversionMovesForTest = probeConversionMoves
var ProbeConversionDirectoryForTest = probeConversionDirectory

var ConversionRecoverForTest = conversionRecover

// SetDirEntryInfoFuncForTest replaces the DirEntry.Info seam used by walks and
// returns a function that restores it.
func SetDirEntryInfoFuncForTest(fn func(fs.DirEntry) (fs.FileInfo, error)) (restore func()) {
	dirEntryInfoFunc = fn
	return func() { dirEntryInfoFunc = nil }
}
