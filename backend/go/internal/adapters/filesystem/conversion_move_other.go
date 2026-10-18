//go:build !linux && !windows

package filesystem

import "github.com/Clementine33/preen/backend/internal/conversion/execute"

func conversionMount(string) (string, error) { return "", execute.ErrMoveUnsupported }
func conversionMove(string, string) (execute.MoveResult, error) {
	return execute.MoveResult{}, execute.ErrMoveUnsupported
}
