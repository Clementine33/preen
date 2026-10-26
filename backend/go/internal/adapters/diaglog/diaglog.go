// Package diaglog is the Diagnostic Log's mechanism: one logfmt handler that
// writes every record to the process's output and keeps the most recent bytes
// in memory, so Log Export hands over exactly what the container captured.
// Nothing is persisted; older lines live only wherever the output went.
package diaglog

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

// capacity bounds the exported tail. Writes grow the buffer up to twice this
// before compacting, so trimming costs one copy per capacity of output.
const capacity = 2 << 20

// Tail is the in-memory end of the Diagnostic Log.
type Tail struct {
	mu  sync.Mutex
	buf []byte
}

// Write keeps p; the handler writes one whole record per call, so the buffer
// only ever holds whole lines.
func (t *Tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > 2*capacity {
		t.buf = append(t.buf[:0], trim(t.buf)...)
	}
	return len(p), nil
}

// Bytes returns a copy of at most capacity bytes of the newest whole lines.
func (t *Tail) Bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return bytes.Clone(trim(t.buf))
}

func trim(buf []byte) []byte {
	if len(buf) <= capacity {
		return buf
	}
	cut := len(buf) - capacity
	if i := bytes.IndexByte(buf[cut:], '\n'); i >= 0 {
		return buf[cut+i+1:]
	}
	return nil
}

// New builds the process logger writing to out and the Tail it also feeds.
// level is PREEN_LOG_LEVEL: empty means info; anything slog cannot parse is
// refused so a typo does not silently hide or flood the log.
func New(out io.Writer, level string) (*slog.Logger, *Tail, error) {
	var minLevel slog.Level
	if level != "" {
		if err := minLevel.UnmarshalText([]byte(level)); err != nil {
			return nil, nil, fmt.Errorf("PREEN_LOG_LEVEL %q: want debug, info, warn or error", level)
		}
	}
	tail := &Tail{}
	// The tail first: it never fails, and a closed output must not cost the export.
	handler := slog.NewTextHandler(io.MultiWriter(tail, out), &slog.HandlerOptions{Level: minLevel})
	return slog.New(handler), tail, nil
}
