package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/onsei/organizer/backend/internal/conversion/execute"
)

// ConversionMoves is conversion's strict move path. General file operations
// retain RenameNoReplace's portable behavior.
type ConversionMoves struct{}

func (ConversionMoves) Move(source, target string) (execute.MoveResult, error) {
	return conversionMove(source, target)
}

func (ConversionMoves) Recover(ctx context.Context, workRoot, source string) (string, execute.MoveResult, error) {
	return conversionRecover(ctx, workRoot, source, conversionMove)
}

func conversionRecover(
	ctx context.Context,
	workRoot, source string,
	move func(string, string) (execute.MoveResult, error),
) (string, execute.MoveResult, error) {
	var result execute.MoveResult
	var attempted string
	dest, err := recoverFile(ctx, workRoot, source, func(src, dst string) error {
		attempted = dst
		var moveErr error
		result, moveErr = move(src, dst)
		if moveErr != nil && (result.Unverified || result.TargetCreated) {
			// Neither collision nor lock retries are safe after a possible landing.
			return &recoveryMoveIncompleteError{cause: moveErr}
		}
		return moveErr
	})
	if err != nil && (result.Unverified || result.TargetCreated) {
		dest = attempted
	}
	return dest, result, err
}

// Probe checks one representative directory per mount, and verifies every
// source/target mount pair. Recovery probing never creates library directories.
func (moves ConversionMoves) Probe(ctx context.Context, requirements []execute.MoveRequirement) error {
	return probeConversionMoves(
		ctx,
		requirements,
		conversionMount,
		func(ctx context.Context, sourceDir, targetDir string) error {
			return probeConversionDirectory(ctx, moves.Move, sourceDir, targetDir)
		},
	)
}

func probeConversionMoves(
	ctx context.Context,
	requirements []execute.MoveRequirement,
	mount func(string) (string, error),
	probe func(context.Context, string, string) error,
) error {
	pairs := map[string][2]string{}
	order := []string{}
	for _, requirement := range requirements {
		if err := ctx.Err(); err != nil {
			return err
		}
		sourceDir := filepath.Dir(filepath.FromSlash(requirement.Source))
		targetDir := filepath.Dir(filepath.FromSlash(requirement.Target))
		if requirement.Recovery {
			parent, rel, err := recoveryDirectory(
				filepath.FromSlash(requirement.WorkRoot),
				filepath.FromSlash(requirement.Source),
			)
			if err != nil {
				return err
			}
			targetDir, err = deepestExistingDirectory(filepath.Join(parent, rel))
			if err != nil {
				return err
			}
		}
		from, err := mount(sourceDir)
		if err != nil {
			return err
		}
		to, err := mount(targetDir)
		if err != nil {
			return err
		}
		if from != to {
			return fmt.Errorf("recovery or publication crosses mounts: %w", execute.ErrMoveUnsupported)
		}
		if _, exists := pairs[from]; !exists {
			order = append(order, from)
			pairs[from] = [2]string{sourceDir, targetDir}
		} else if requirement.Recovery {
			pairs[from] = [2]string{sourceDir, targetDir}
		}
	}
	for _, mount := range order {
		pair := pairs[mount]
		if err := probe(ctx, pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

func deepestExistingDirectory(dir string) (string, error) {
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", err
		}
		return deepestExistingDirectory(parent)
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("probe directory is not a real directory: %s", dir)
	}
	return dir, nil
}

func probeConversionDirectory(
	ctx context.Context,
	move func(string, string) (execute.MoveResult, error),
	sourceDir, targetDir string,
) (failure error) {
	name := "conversion.tmp." + uuid.NewString()
	paths := []string{
		filepath.Join(sourceDir, name+"-source"),
		filepath.Join(targetDir, name+"-occupied"),
		filepath.Join(targetDir, name+"-target"),
	}
	owned := map[string]os.FileInfo{}
	defer func() {
		for _, path := range paths {
			info, err := os.Lstat(path)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil || owned[path] == nil || !os.SameFile(owned[path], info) {
				failure = errors.Join(failure, fmt.Errorf("probe scratch left unverified: %s", path))
				continue
			}
			if err := os.Remove(path); err != nil {
				failure = errors.Join(failure, fmt.Errorf("probe scratch remains %s: %w", path, err))
			}
		}
	}()
	for _, path := range paths[:2] {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err != nil {
			return err
		}
		info, err := file.Stat()
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return errors.Join(err, closeErr)
		}
		owned[path] = info
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	result, err := move(paths[0], paths[1])
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	if result.Unverified {
		return fmt.Errorf("filesystem refusal outcome unverified: %w", err)
	}
	if !errors.Is(err, fs.ErrExist) || result.TargetCreated || result.SourceRemoved {
		return fmt.Errorf("filesystem failed refusal of occupied target: %w", execute.ErrMoveUnsupported)
	}
	if cancelErr := ctx.Err(); cancelErr != nil {
		return cancelErr
	}
	result, err = move(paths[0], paths[2])
	if result.TargetCreated || result.Unverified {
		owned[paths[2]] = owned[paths[0]]
	}
	if err != nil {
		return err
	}
	if !result.TargetCreated || !result.SourceRemoved {
		return execute.ErrMoveUnsupported
	}
	return nil
}

// recoveryMoveIncompleteError intentionally does not unwrap the collision errno.
// Generic recovery recognizes it before either collision or lock retry logic.
type recoveryMoveIncompleteError struct{ cause error }

func (e *recoveryMoveIncompleteError) Error() string {
	return "recovery move incomplete: " + e.cause.Error()
}
