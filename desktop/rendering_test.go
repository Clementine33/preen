package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// The child process observes the environment after every package, Wails
// included, has initialized.
func TestDMABUFRendererSurvivesWailsInit(t *testing.T) {
	if want, child := os.LookupEnv("PREEN_TEST_WANT_DMABUF"); child {
		if got := os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER"); got != want {
			t.Fatalf("WEBKIT_DISABLE_DMABUF_RENDERER = %q, want %q", got, want)
		}
		return
	}
	if runtime.GOOS != "linux" {
		t.Skip("WebKitGTK only")
	}
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "WEBKIT_DISABLE_DMABUF_RENDERER=") {
			env = append(env, kv)
		}
	}
	for _, c := range []struct{ inherited, want []string }{
		{nil, []string{"PREEN_TEST_WANT_DMABUF=0"}},
		{[]string{"WEBKIT_DISABLE_DMABUF_RENDERER=1"}, []string{"PREEN_TEST_WANT_DMABUF=1"}},
	} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestDMABUFRendererSurvivesWailsInit$")
		cmd.Env = append(append(append([]string{}, env...), c.inherited...), c.want...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("inherited %v: %v\n%s", c.inherited, err, out)
		}
	}
}
