//go:build linux

package filesystem

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/onsei/organizer/backend/internal/conversion/execute"
)

func conversionMove(source, target string) (execute.MoveResult, error) {
	return conversionMoveOnMounts(source, target, conversionMount)
}

func conversionMoveOnMounts(source, target string, mount func(string) (string, error)) (execute.MoveResult, error) {
	from, err := mount(filepath.Dir(source))
	if err != nil {
		return execute.MoveResult{}, err
	}
	to, err := mount(filepath.Dir(target))
	if err != nil {
		return execute.MoveResult{}, err
	}
	if from != to {
		return execute.MoveResult{}, fmt.Errorf("cross-mount move: %w", execute.ErrMoveUnsupported)
	}
	err = unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE)
	if err == nil {
		return execute.MoveResult{TargetCreated: true, SourceRemoved: true}, nil
	}
	if errors.Is(err, unix.EXDEV) {
		return execute.MoveResult{}, fmt.Errorf("cross-mount move: %w", execute.ErrMoveUnsupported)
	}
	if !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.ENOSYS) && !errors.Is(err, unix.EOPNOTSUPP) {
		if errors.Is(err, fs.ErrExist) || errors.Is(err, fs.ErrPermission) {
			return execute.MoveResult{}, err
		}
		// Network failures may leave the successful request's reply unknown.
		return execute.MoveResult{Unverified: true}, err
	}
	return conversionLinkMove(source, target, os.Link, os.Remove, os.Lstat)
}

func conversionLinkMove(
	source, target string,
	link func(string, string) error,
	unlink func(string) error,
	inspect func(string) (os.FileInfo, error),
) (execute.MoveResult, error) {
	expected, err := inspect(source)
	if err != nil {
		return execute.MoveResult{}, err
	}
	if !expected.Mode().IsRegular() {
		return execute.MoveResult{}, execute.ErrMoveUnsupported
	}
	err = link(source, target)
	if errors.Is(err, fs.ErrExist) {
		src, srcErr := inspect(source)
		dst, dstErr := inspect(target)
		if srcErr != nil || dstErr != nil {
			return execute.MoveResult{Unverified: true}, fmt.Errorf("link outcome unverified: %w", err)
		}
		if !os.SameFile(expected, dst) {
			return execute.MoveResult{}, err
		}
		if !os.SameFile(expected, src) {
			return execute.MoveResult{
				TargetCreated: true,
				Unverified:    true,
			}, fmt.Errorf(
				"linked source name changed: %w",
				err,
			)
		}
		err = nil // NFS may return EEXIST for its retransmitted successful link.
	}
	if err != nil {
		if errors.Is(err, unix.EXDEV) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOSYS) {
			return execute.MoveResult{}, fmt.Errorf("hard link unavailable: %w", execute.ErrMoveUnsupported)
		}
		if errors.Is(err, fs.ErrPermission) {
			return execute.MoveResult{}, err
		}
		return execute.MoveResult{Unverified: true}, err
	}
	src, srcErr := inspect(source)
	dst, dstErr := inspect(target)
	if srcErr != nil || dstErr != nil || !os.SameFile(expected, src) || !os.SameFile(expected, dst) {
		return execute.MoveResult{Unverified: true}, fmt.Errorf("created link identity unverified")
	}
	result := execute.MoveResult{TargetCreated: true}
	if err := unlink(source); err != nil {
		return result, err
	}
	result.SourceRemoved = true
	return result, nil
}
