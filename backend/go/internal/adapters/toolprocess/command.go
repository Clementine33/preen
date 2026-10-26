// Package toolprocess constructs external tool commands for adapters. Windows
// creation flags belong here because a parent's no-console flag is not inherited.
package toolprocess

import (
	"context"
	"os/exec"
)

// CommandContext constructs a tool command with the platform's window policy.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	configureWindow(cmd)
	return cmd
}
