//go:build unix

package media_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/media"
)

// stalledFile is a FIFO with no writer: opening it for reading blocks, the way
// a read on a stalled share does. Cleanup opens and closes a writer so the
// abandoned read ends.
func stalledFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close()
		}
	})
	return filepath.ToSlash(path)
}

func TestProbesGiveUpOnAStalledRead(t *testing.T) {
	p := media.New("")
	p.Timeout = 100 * time.Millisecond
	for _, name := range []string{"cover.png", "track.mp3"} {
		t.Run(name, func(t *testing.T) {
			path := stalledFile(t, name)
			started := time.Now()
			var err error
			if strings.HasSuffix(name, ".png") {
				_, _, err = p.ImageDimensions(context.Background(), path)
			} else {
				_, _, err = p.AudioFacts(context.Background(), path)
			}
			if err == nil || !strings.Contains(err.Error(), "timed out") {
				t.Errorf("err = %v, want a timeout", err)
			}
			if elapsed := time.Since(started); elapsed > 2*time.Second {
				t.Errorf("probe returned after %s", elapsed)
			}
		})
	}
}

func TestProbeStopsAHungFFprobe(t *testing.T) {
	dir := t.TempDir()
	hung := filepath.Join(dir, "ffprobe")
	if err := os.WriteFile(hung, []byte("#!/bin/sh\nexec sleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	audio := filepath.Join(dir, "track.flac") // not native: always asks ffprobe
	if err := os.WriteFile(audio, []byte("fLaC"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := media.New(hung)
	p.Timeout = 200 * time.Millisecond
	started := time.Now()
	_, _, err := p.AudioFacts(context.Background(), filepath.ToSlash(audio))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v, want a timeout", err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Errorf("probe returned after %s", elapsed)
	}
}
