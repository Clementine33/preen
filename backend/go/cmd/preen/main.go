// Command preen is the process entry point: it wires the
// process's signals and working directory and hands the rest to internal/app,
// which owns assembly and the process's lifetime.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Clementine33/preen/backend/internal/app"
)

// version is the single default development version. Release builds override it
// with -X main.version; the startup handshake passes it to the client.
var version = "0.2.0-dev"

func main() {
	watchStdin := flag.Bool("watch-stdin", false, "shut down gracefully when stdin reaches EOF")
	allowLAN := flag.Bool("allow-lan", false, "override server.allow_lan")
	port := flag.Int("port", 0, "override server.port")
	webDir := flag.String("web-dir", "", "serve the built web application from this directory")
	flag.Parse()
	if flag.NArg() != 0 {
		// A usage error, like the ones flag itself reports.
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(2)
	}
	var allowLANOverride *bool
	var portOverride *int
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "allow-lan":
			allowLANOverride = allowLAN
		case "port":
			portOverride = port
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	if *watchStdin {
		go app.WatchStdinEOF(ctx, os.Stdin, cancel)
	}

	// Resolve the data directory relative to the executable, so the binary works
	// wherever it is unpacked; PREEN_DATA_DIR overrides it.
	exePath, err := os.Executable()
	if err != nil {
		slog.Error("cannot find executable path", "err", err)
		//nolint:gocritic // a process that cannot locate itself exits, deferred cleanup and all
		os.Exit(1)
	}
	// Executable is at backend/go/bin/preen[.exe]; the data
	// directory is the project root, three levels up.
	dataDir := filepath.Join(filepath.Dir(exePath), "..", "..", "..")
	dataDir, _ = filepath.Abs(dataDir)
	if d := os.Getenv("PREEN_DATA_DIR"); d != "" {
		dataDir = d
	}

	app.Run(ctx, app.Config{
		DataDir: dataDir,
		// The configuration file lives beside the database.
		ConfigDir: dataDir,
		Token:     os.Getenv("PREEN_TOKEN"),
		Version:   version,
		LogLevel:  os.Getenv("PREEN_LOG_LEVEL"),
		AllowLAN:  allowLANOverride,
		Port:      portOverride,
		WebDir:    *webDir,
	})
}
