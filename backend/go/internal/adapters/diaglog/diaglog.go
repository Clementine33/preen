// Package diaglog is the Diagnostic Log's mechanism: one logfmt handler that
// writes every record to the process's output and keeps the most recent lines
// in memory, so Log Export hands over exactly what the container captured.
// Nothing is persisted; older lines live only wherever the output went.
package diaglog

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// capacity bounds the bytes the Tail holds and exports.
const capacity = 2 << 20

// Tail is the in-memory end of the Diagnostic Log: the newest whole lines
// whose total size fits capacity.
type Tail struct {
	mu    sync.Mutex
	lines [][]byte
	size  int
}

// Write keeps p as one line; the handler writes one whole record per call.
// The oldest lines go first to make room, one whole line at a time. A record
// larger than the whole Tail cannot be kept whole, so it is skipped and the
// lines before it stay; stdout still has it.
func (t *Tail) Write(p []byte) (int, error) {
	if len(p) > capacity {
		return len(p), nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lines = append(t.lines, bytes.Clone(p))
	t.size += len(p)
	for t.size > capacity {
		t.size -= len(t.lines[0])
		t.lines[0] = nil // release the bytes, not just the slot
		t.lines = t.lines[1:]
	}
	return len(p), nil
}

// Bytes returns a copy of the kept lines, oldest first.
func (t *Tail) Bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return bytes.Join(t.lines, nil)
}

// levels are the PREEN_LOG_LEVEL values. slog's own parser also takes offsets
// such as ERROR+1, which would silently drop every error, so only these pass.
var levels = map[string]slog.Level{
	"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError,
}

// New builds the process logger writing to out and the Tail it also feeds.
// level is PREEN_LOG_LEVEL: empty means info; anything else unknown is refused
// so a typo does not silently hide or flood the log.
func New(out io.Writer, level string) (*slog.Logger, *Tail, error) {
	minLevel := slog.LevelInfo
	if level != "" {
		var ok bool
		if minLevel, ok = levels[strings.ToLower(level)]; !ok {
			return nil, nil, fmt.Errorf("PREEN_LOG_LEVEL %q: want debug, info, warn or error", level)
		}
	}
	tail := &Tail{}
	// The tail first: it never fails, and a closed output must not cost the export.
	handler := slog.NewTextHandler(io.MultiWriter(tail, out), &slog.HandlerOptions{Level: minLevel})
	return slog.New(handler), tail, nil
}
