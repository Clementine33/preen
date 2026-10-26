package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
)

// WatchStdinEOF requests graceful shutdown when a host closes its stdin pipe.
func WatchStdinEOF(ctx context.Context, stdin io.Reader, cancel context.CancelFunc) {
	if stdin == nil {
		return
	}

	buf := make([]byte, 1)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := stdin.Read(buf)
		if errors.Is(err, io.EOF) {
			cancel()
			return
		}
		if err != nil {
			slog.Error("stdin watcher read failed", "err", err)
			return
		}
		if n == 0 {
			continue
		}
	}
}
