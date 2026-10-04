package execute

import (
	"context"
	"errors"
)

var ErrMoveUnsupported = errors.New("safe filesystem move unsupported")

// MoveResult describes facts even when a move returns an error. Unverified
// names must be protected from generic cleanup because either may be the copy.
type MoveResult struct {
	TargetCreated bool
	SourceRemoved bool
	Unverified    bool
}

// MoveRequirement names a real session move, or recovery of Source under WorkRoot.
type MoveRequirement struct {
	WorkRoot, Source, Target string
	Recovery                 bool
}

// FileMoves owns strict publication, recovery layout and capability probing.
// Conversion never falls back to a check followed by an overwriting rename.
type FileMoves interface {
	Move(source, target string) (MoveResult, error)
	Recover(context.Context, string, string) (string, MoveResult, error)
	Probe(context.Context, []MoveRequirement) error
}
