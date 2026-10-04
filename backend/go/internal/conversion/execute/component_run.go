package execute

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
)

// ToolsConfig represents the tools configuration for conversion.
type ToolsConfig struct {
	FFmpegPath  string
	FFprobePath string
}

// DeleteMode selects how obsolete files leave their place. Soft deletion keeps
// removed files beside their work root; it is never downgraded to a hard
// delete when recovery is unavailable.
type DeleteMode string

// RecoverFile moves a file into the recovery directory beside its work root.
type RecoverFile func(ctx context.Context, workRoot, source string) (string, error)

const (
	DeleteModeSoft DeleteMode = "soft"
	DeleteModeHard DeleteMode = "hard"
)

// Component run stages, reported through ComponentRunResult.Stage.
const (
	ComponentStagePrecheck    = "precheck"
	ComponentStageMaterialize = "materialize"
	ComponentStageValidate    = "validate"
	ComponentStageCommit      = "commit"
	ComponentStageRemove      = "remove"
)

// Component run statuses.
const (
	ComponentStatusSucceeded = "succeeded"
	ComponentStatusFailed    = "failed"
	ComponentStatusCanceled  = "canceled"
)

// Component failure codes reported through ComponentError.Code. They name the
// gate that refused or interrupted the component, not a specific errno.
const (
	ComponentCodeInvalidRequest    = "COMPONENT_INVALID_REQUEST"
	ComponentCodeBlocked           = "COMPONENT_BLOCKED"
	ComponentCodeUnsupportedOp     = "COMPONENT_UNSUPPORTED_OPERATION"
	ComponentCodeInvalidDependency = "COMPONENT_INVALID_DEPENDENCY"
	ComponentCodeConflict          = "COMPONENT_TARGET_CONFLICT"
	ComponentCodeFileChanged       = "COMPONENT_FILE_CHANGED"
	ComponentCodePathUnsafe        = "COMPONENT_PATH_UNSAFE"
	ComponentCodeToolsUnavailable  = "COMPONENT_TOOLS_UNAVAILABLE"
	ComponentCodeEncodeFailed      = "COMPONENT_ENCODE_FAILED"
	ComponentCodeValidateFailed    = "COMPONENT_VALIDATE_FAILED"
	ComponentCodeCommitFailed      = "COMPONENT_COMMIT_FAILED"
	ComponentCodeDeleteFailed      = "COMPONENT_DELETE_FAILED"
	ComponentCodeCanceled          = "COMPONENT_CANCELED"
)

// ComponentRunRequest executes one frozen Component against its member root.
// The component, the effective target specs of its partition and the root are
// consumed exactly as frozen; no reconciliation or live draft state is
// consulted.
type ComponentRunRequest struct {
	Root       string
	Component  reconcile.ComponentOutcome
	Specs      reconcile.DesiredProfile
	DeleteMode DeleteMode
	// Encoder is the media tool this run probes and writes with. The pipeline
	// never builds one itself: it is handed the encoder the process is
	// configured with, so a run can be driven by a test double as well.
	Encoder Encoder
	// Recover is required for soft deletion and protects replaced output files.
	Recover    RecoverFile
	WithinRoot ResolvedWithinRoot
	Moves      FileMoves
	Files      FileHandles
}

// ResolvedWithinRoot checks containment after host filesystem indirection.
// It is the execute consumer's single filesystem-resolution operation.
type ResolvedWithinRoot func(root, candidate string) (bool, error)

// ComponentRunResult reports what actually happened to one Component. On a
// failed or canceled run the lists describe the partial outcome; file changes
// are never rolled back globally, so callers must reconcile these facts with
// the disk.
type ComponentRunResult struct {
	// Status is succeeded, failed or canceled.
	Status  string
	Outputs []OutputFacts
	// Unverified reports a move whose on-disk outcome could not be established.
	Unverified bool
	// Stage is the stage the run stopped in; empty on success and when the
	// canceled request never entered a stage.
	Stage string
	// Committed are the target paths that now hold new outputs, in commit order.
	Committed []string
	// Removed are the obsolete paths removed from their place, in removal order.
	Removed []string
	// Remaining are the operation surfaces not completed, in frozen order:
	// target paths of uncommitted encodes, then paths of unremoved files.
	Remaining []string
	// Recovery lists paths that still hold media needing operator attention:
	// soft-delete destinations, replaced-old-file copies and temporary files
	// that could not be cleaned. All entries are persisted-form paths.
	Recovery []string
}

// ComponentError is the stable failure cause of a component run.
type ComponentError struct {
	Stage   string
	Code    string
	Path    string
	Message string
	Err     error
}

// Error implements error.
func (e *ComponentError) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

// Unwrap returns the underlying cause.
func (e *ComponentError) Unwrap() error { return e.Err }

// componentToolkit is the component-run IO surface. Function fields keep every
// filesystem or encoder failure injectable without mocks.
type componentToolkit struct {
	encode      func(ctx context.Context, src, dst *os.File, spec reconcile.AudioOutputSpec) (AudioStream, error)
	validate    func(context.Context, *os.File, reconcile.AudioOutputSpec, AudioStream) (ValidatedOutput, error)
	move        func(oldpath, newpath string) (MoveResult, error)
	remove      func(path string) error
	recoverMove func(context.Context, string, string) (string, MoveResult, error)
	unverified  bool
}

// defaultComponentToolkit returns the real encoder and filesystem operations.
func defaultComponentToolkit(req ComponentRunRequest) *componentToolkit {
	tk := &componentToolkit{remove: os.Remove}
	if req.Encoder != nil {
		tk.encode = req.Encoder.Encode
		tk.validate = req.Encoder.ValidateOutput
	}
	if req.Moves != nil {
		tk.move, tk.recoverMove = req.Moves.Move, req.Moves.Recover
	} else {
		tk.move = func(string, string) (MoveResult, error) { return MoveResult{}, ErrMoveUnsupported }
		tk.recoverMove = func(context.Context, string, string) (string, MoveResult, error) {
			return "", MoveResult{}, ErrMoveUnsupported
		}
	}
	return tk
}

// RunComponent executes one frozen Component: every output is materialized and
// validated before any commit, and every commit succeeds before any obsolete
// file is removed, so old audio is never cleaned while a replacement is still
// uncommitted. It takes no cross-run lock: callers must serialize runs over
// the same or overlapping roots (the execution session layer owns that).
func RunComponent(ctx context.Context, req ComponentRunRequest) (ComponentRunResult, error) {
	return runComponent(ctx, req, defaultComponentToolkit(req))
}

func runComponent(ctx context.Context, req ComponentRunRequest, tk *componentToolkit) (ComponentRunResult, error) {
	prepared, err := prepareComponent(ctx, req, tk)
	if err != nil {
		return stoppedResult(err), err
	}
	for i := range prepared.Encodes() {
		if encErr := prepared.EncodeOne(ctx, i); encErr != nil {
			return prepared.Discard(encErr), encErr
		}
	}
	return prepared.Commit(ctx)
}

// stopRun finalizes a failed or canceled run: it records the stage and the
// not-completed operations and removes this run's unconsumed temporary
// outputs, reporting any leftover path that could not be cleaned.
func stopRun(
	plan *plannedComponent, tk *componentToolkit, result ComponentRunResult, status, stage string, cause error,
) (ComponentRunResult, error) {
	result.Unverified = tk.unverified
	result.Status = status
	result.Stage = stage
	result.Remaining = remainingOps(plan, len(result.Committed), len(result.Removed))
	result.Recovery = append(result.Recovery, cleanupTemps(tk, plan)...)
	return result, cause
}

// stopCanceled finalizes a canceled run at a stage boundary.
func stopCanceled(
	plan *plannedComponent, tk *componentToolkit, result ComponentRunResult, stage string, ctxErr error,
) (ComponentRunResult, error) {
	return stopRun(plan, tk, result, ComponentStatusCanceled, stage, canceledComponentError(stage, ctxErr))
}

// remainingOps lists the operation surfaces not completed: the targets of
// uncommitted encodes followed by the paths of unremoved files, both in frozen
// order and persisted form.
func remainingOps(plan *plannedComponent, committed, removed int) []string {
	remaining := make([]string, 0, len(plan.encodes)-committed+len(plan.removes)-removed)
	for _, enc := range plan.encodes[committed:] {
		remaining = append(remaining, posixForm(enc.target))
	}
	for _, rem := range plan.removes[removed:] {
		remaining = append(remaining, posixForm(rem.source))
	}
	return remaining
}

// cleanupTemps removes this run's uncommitted temporary outputs. A path that
// cannot be cleaned is reported instead of silently dropped.
func cleanupTemps(tk *componentToolkit, plan *plannedComponent) []string {
	var left []string
	for i := range plan.encodes {
		temp := plan.encodes[i].temp
		if plan.encodes[i].protected {
			left = append(left, posixForm(temp))
			continue
		}
		if temp == "" {
			continue
		}
		info, inspectErr := os.Lstat(temp)
		if errors.Is(inspectErr, fs.ErrNotExist) {
			continue
		}
		if inspectErr != nil || plan.encodes[i].artifact == nil || !os.SameFile(plan.encodes[i].artifact, info) {
			left = append(left, posixForm(temp))
			continue
		}
		if err := tk.remove(temp); err != nil && !errors.Is(err, fs.ErrNotExist) {
			left = append(left, posixForm(temp))
		}
	}
	return left
}

// canceledComponentError builds the error of a canceled run.
func canceledComponentError(stage string, cause error) *ComponentError {
	return &ComponentError{Stage: stage, Code: ComponentCodeCanceled, Message: "component run canceled", Err: cause}
}

// observeMoves remembers uncertainty without allowing later cleanup to claim a
// complete inventory observation. Moves only run in the serial commit phase.
func (tk *componentToolkit) observeMoves() {
	move, recoverMove := tk.move, tk.recoverMove
	tk.move = func(src, dst string) (MoveResult, error) {
		result, err := move(src, dst)
		tk.unverified = tk.unverified || result.Unverified
		return result, err
	}
	tk.recoverMove = func(ctx context.Context, root, src string) (string, MoveResult, error) {
		path, result, err := recoverMove(ctx, root, src)
		tk.unverified = tk.unverified || result.Unverified
		return path, result, err
	}
}
