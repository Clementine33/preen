//go:build !windows

package toolprocess

import "os/exec"

func configureWindow(_ *exec.Cmd) {}
