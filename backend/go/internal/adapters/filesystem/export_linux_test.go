//go:build linux

package filesystem //nolint:testpackage // Narrow Linux-only test seam; external tests exercise behavior.

import (
	"os"

	"github.com/onsei/organizer/backend/internal/conversion/execute"
)

var MountFromInfoForTest = mountFromInfo
var ConversionMountForTest = conversionMount
var ConversionMoveOnMountsForTest = conversionMoveOnMounts

func ConversionLinkMoveForTest(
	source, target string,
	link func(string, string) error,
	unlink func(string) error,
	inspect func(string) (os.FileInfo, error),
) (execute.MoveResult, error) {
	return conversionLinkMove(source, target, link, unlink, inspect)
}

var ConversionMountFromStatForTest = conversionMountFromStat
