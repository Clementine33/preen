package toolprocess

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}
