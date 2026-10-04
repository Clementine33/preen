//go:build !linux && !windows

package filesystem

import "github.com/onsei/organizer/backend/internal/conversion/execute"

func conversionMount(string) (string, error) { return "", execute.ErrMoveUnsupported }
func conversionMove(string, string) (execute.MoveResult, error) {
	return execute.MoveResult{}, execute.ErrMoveUnsupported
}
