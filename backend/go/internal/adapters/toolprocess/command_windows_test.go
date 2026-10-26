package toolprocess_test

import (
	"testing"

	"golang.org/x/sys/windows"

	"github.com/Clementine33/preen/backend/internal/adapters/toolprocess"
)

func TestCommandContextCreatesNoWindow(t *testing.T) {
	cmd := toolprocess.CommandContext(t.Context(), "ffmpeg", "-version")
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatal("tool command must use CREATE_NO_WINDOW")
	}
}
