package conversion

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/Clementine33/preen/backend/internal/conversion/execute"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func executionRequirements(
	in workset.ExecutionSessionInput,
) ([]execute.MoveRequirement, []reconcile.AudioOutputSpec, error) {
	mode, err := deleteModeFromOptions(in.Options)
	if err != nil {
		return nil, nil, err
	}
	moves := []execute.MoveRequirement{}
	specs := []reconcile.AudioOutputSpec{}
	containers := map[reconcile.Codec]bool{}
	for _, unit := range in.Units {
		var outcome reconcile.ComponentOutcome
		if err := json.Unmarshal(in.Outcomes[unit.Index], &outcome); err != nil {
			return nil, nil, err
		}
		var profile reconcile.DesiredProfile
		if err := json.Unmarshal(unit.Payload, &profile); err != nil {
			return nil, nil, err
		}
		files := map[string]bool{}
		for _, file := range outcome.Files {
			files[file.Path] = true
		}
		for _, op := range outcome.Operations {
			if op.Kind == reconcile.OpKindEncode {
				moves = append(moves, execute.MoveRequirement{Source: op.TargetPath, Target: op.TargetPath})
				for _, spec := range []*reconcile.AudioOutputSpec{profile.Lossless, profile.Encoded} {
					if spec == nil ||
						reconcile.ExtForCodec(spec.Codec) != strings.ToLower(filepath.Ext(op.TargetPath)) ||
						containers[spec.Codec] {
						continue
					}
					containers[spec.Codec] = true
					specs = append(specs, *spec)
				}
			}
			if mode == execute.DeleteModeSoft &&
				(op.Kind == reconcile.OpKindRemoveObsolete || (op.Kind == reconcile.OpKindEncode && files[op.TargetPath])) {
				source := op.SourcePath
				if op.Kind == reconcile.OpKindEncode {
					source = op.TargetPath
				}
				moves = append(moves, execute.MoveRequirement{WorkRoot: unit.RootPath, Source: source, Recovery: true})
			}
		}
	}
	return moves, specs, nil
}

// sessionEncoder delegates media work while Check and Version are session facts.
type sessionEncoder struct {
	execute.Encoder

	version string
}

func (*sessionEncoder) Check() error                     { return nil }
func (s *sessionEncoder) Version(context.Context) string { return s.version }

func (t *Task) configureExecution(
	ctx context.Context,
	moves []execute.MoveRequirement,
	specs []reconcile.AudioOutputSpec,
) (*Task, error) {
	session := *t
	session.sessionFixed = true
	var err error
	if len(moves) > 0 {
		if t.moves == nil {
			err = execute.ErrMoveUnsupported
		} else {
			err = t.moves.Probe(ctx, moves)
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, workset.NewError(
					workset.ErrKindInternal,
					"CANCELED",
					"execution canceled: "+err.Error(),
					errors.Join(ctx.Err(), err),
				)
			}
			code := "FILESYSTEM_PRECHECK_FAILED"
			if errors.Is(err, execute.ErrMoveUnsupported) {
				code = "FILESYSTEM_OPERATION_UNSUPPORTED"
			}
			return nil, workset.NewError(workset.ErrKindInternal, code, "conversion not started: "+err.Error(), err)
		}
	}
	if len(specs) == 0 {
		session.sessionEncoder = nil
		session.buildEncoder = func() execute.Encoder { return nil }
		return &session, nil
	}
	encoder := t.buildEncoder()
	if encoder == nil {
		return nil, workset.NewError(
			workset.ErrKindInternal,
			execute.ComponentCodeToolsUnavailable,
			"encode tools unavailable",
			nil,
		)
	}
	if err := encoder.Check(); err != nil {
		return nil, unitFailureOf(
			&execute.ComponentError{
				Stage:   execute.ComponentStagePrecheck,
				Code:    execute.ComponentCodeToolsUnavailable,
				Message: "encode tools unavailable",
				Err:     err,
			},
		)
	}
	version := encoder.Version(ctx)
	if err := encoder.CheckFD(ctx, specs); err != nil {
		if ctx.Err() != nil {
			return nil, workset.NewError(
				workset.ErrKindInternal,
				"CANCELED",
				"execution canceled: "+err.Error(),
				errors.Join(ctx.Err(), err),
			)
		}
		return nil, unitFailureOf(
			&execute.ComponentError{
				Stage:   execute.ComponentStagePrecheck,
				Code:    execute.ComponentCodeToolsUnavailable,
				Message: "FD encoding unavailable",
				Err:     err,
			},
		)
	}
	session.sessionEncoder = &sessionEncoder{Encoder: encoder, version: version}
	return &session, nil
}
