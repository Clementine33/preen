//go:build windows

package filesystem

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/Clementine33/preen/backend/internal/conversion/execute"
)

func conversionMount(path string) (string, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	const capacity = 32768
	buffer := make([]uint16, capacity)
	if err := windows.GetVolumePathName(pathPtr, &buffer[0], capacity); err != nil {
		return "", err
	}
	mount := windows.UTF16ToString(buffer)
	mountPtr, err := windows.UTF16PtrFromString(mount)
	if err != nil {
		return "", err
	}
	var serial uint32
	if volumeErr := windows.GetVolumeInformation(mountPtr, nil, 0, &serial, nil, nil, nil, 0); volumeErr != nil {
		return "", volumeErr
	}
	return fmt.Sprintf("%08x|%s", serial, strings.ToLower(mount)), nil
}

func conversionMove(source, target string) (execute.MoveResult, error) {
	from, err := conversionMount(filepath.Dir(source))
	if err != nil {
		return execute.MoveResult{}, err
	}
	to, err := conversionMount(filepath.Dir(target))
	if err != nil {
		return execute.MoveResult{}, err
	}
	if from != to {
		return execute.MoveResult{}, fmt.Errorf("cross-mount move: %w", execute.ErrMoveUnsupported)
	}
	if err := RenameNoReplace(source, target); err != nil {
		return execute.MoveResult{}, err
	}
	return execute.MoveResult{TargetCreated: true, SourceRemoved: true}, nil
}
