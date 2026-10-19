package e2e //nolint:testpackage // shares the process harness with the HTTP integration tests

import (
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func TestBackendProcessLifecycle(t *testing.T) {
	bin := buildBackendBinary(t)
	t.Run("stdin watcher is explicit", func(t *testing.T) {
		proc := startBackendBinary(t, bin, t.TempDir(), "secret-not-in-output", "--watch-stdin")
		if err := proc.stdin.Close(); err != nil {
			t.Fatal(err)
		}
		awaitBackendExit(t, proc)
	})
	if runtime.GOOS == "windows" {
		return
	} // Windows hosts request graceful shutdown through stdin.
	for _, signal := range []os.Signal{syscall.SIGTERM, os.Interrupt} {
		t.Run(signal.String(), func(t *testing.T) {
			proc := startBackendBinary(t, bin, t.TempDir(), "secret-not-in-output")
			if err := proc.stdin.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-proc.done:
				t.Fatalf("standalone process exited on stdin EOF: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			if err := proc.cmd.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			awaitBackendExit(t, proc)
		})
	}
}

func awaitBackendExit(t *testing.T, proc backendProc) {
	t.Helper()
	select {
	case err := <-proc.done:
		if err != nil {
			t.Fatalf("graceful shutdown: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("backend did not shut down")
	}
}
