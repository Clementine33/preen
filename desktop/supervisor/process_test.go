package supervisor_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Clementine33/preen/desktop/supervisor"
)

func TestHelperProcess(t *testing.T) {
	mode := os.Getenv("PREEN_SUPERVISOR_HELPER")
	if mode == "" {
		return
	}
	fmt.Fprintln(os.Stderr, "before readiness: diagnostic")
	if mode == "early" {
		os.Exit(3)
	}
	if mode == "silent" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	if mode == "long" {
		fmt.Fprintln(os.Stdout, strings.Repeat("x", 100000))
	}
	fmt.Fprintln(os.Stdout, "PREEN_BACKEND_READY version=0.2.0 http_port=43210")
	if mode == "crash" {
		fmt.Fprintln(os.Stderr, "crash reason")
		os.Exit(7)
	}
	if mode == "stuck" {
		for {
			time.Sleep(time.Hour)
		}
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	fmt.Fprintln(os.Stderr, "stdin closed: orderly exit")
	os.Exit(0)
}

func TestUnexpectedExitsKeepDiagnostics(t *testing.T) {
	for _, mode := range []string{"early", "crash"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			p, err := supervisor.Start(ctx, helperPlan(t, mode))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = p.Stop() })
			_, readyErr := p.ResolveAPI(ctx)
			err = p.Wait()
			var exit *supervisor.UnexpectedExit
			if !errors.As(err, &exit) || !strings.Contains(exit.Tail, "before readiness: diagnostic") {
				t.Fatalf("exit lacks diagnostics: %v", err)
			}
			if mode == "early" && readyErr == nil {
				t.Fatal("exit before handshake must fail readiness")
			}
			if mode == "crash" && !strings.Contains(exit.Tail, "crash reason") {
				t.Fatal("missing crash reason")
			}
		})
	}
}

func TestVersionMismatchAndCanceledReadinessReleaseChild(t *testing.T) {
	for _, mode := range []string{"orderly", "silent"} {
		t.Run(mode, func(t *testing.T) {
			plan := helperPlan(t, mode)
			plan.Version = "0.3.0"
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			p, err := supervisor.Start(context.Background(), plan)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = p.Stop() })
			_, err = p.ResolveAPI(ctx)
			if mode == "orderly" && (err == nil || !strings.Contains(err.Error(), "does not match")) {
				t.Fatalf("mismatch: %v", err)
			}
			if mode == "silent" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("cancellation: %v", err)
			}
			done := make(chan struct{})
			go func() { _ = p.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("failed readiness left child alive")
			}
		})
	}
}

func TestStopForcesUnresponsiveChildAfterTenSeconds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	p, err := supervisor.Start(ctx, helperPlan(t, "stuck"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop() })
	if _, err = p.ResolveAPI(ctx); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err = p.Stop(); err == nil || !strings.Contains(err.Error(), "forced termination") {
		t.Fatalf("force result: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 10*time.Second || elapsed > 12*time.Second {
		t.Fatalf("shutdown took %v", elapsed)
	}
}

func TestLongOutputIsLoggedWithoutBlockingReadiness(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	plan := helperPlan(t, "long")
	p, err := supervisor.Start(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop() })
	if _, err = p.ResolveAPI(ctx); err != nil {
		t.Fatal(err)
	}
	if err = p.Stop(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(plan.DataDir + "/logs/preen.log")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), strings.Repeat("x", 100000)) {
		t.Fatal("long stdout line was truncated")
	}
}

func TestStoppedBeforeHandshakeCannotResolveAnEmptyAPI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p, err := supervisor.Start(ctx, helperPlan(t, "silent"))
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err = p.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err = p.ResolveAPI(context.Background()); err == nil {
		t.Fatal("stopped service must not resolve successfully with empty credentials")
	}
}

func helperPlan(t *testing.T, mode string) supervisor.Plan {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return supervisor.Plan{Executable: exe, Args: []string{"-test.run=^TestHelperProcess$"}, Env: append(os.Environ(), "PREEN_SUPERVISOR_HELPER="+mode, "GORACE=atexit_sleep_ms=0"), DataDir: t.TempDir(), Token: "page-key", Version: "0.2.0"}
}

func TestProcessReadinessAndOrderlyStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	plan := helperPlan(t, "orderly")
	p, err := supervisor.Start(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop() })
	api, err := p.ResolveAPI(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if api.BaseURL != "http://127.0.0.1:43210/api/v1" || api.Token != "page-key" {
		t.Fatalf("page credentials: %+v", api)
	}
	if err = p.Stop(); err != nil {
		t.Fatal(err)
	}
	if err = p.Wait(); err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(plan.DataDir + "/logs/preen.log")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"before readiness: diagnostic", "PREEN_BACKEND_READY", "stdin closed: orderly exit"} {
		if !strings.Contains(string(log), want) {
			t.Fatalf("log missing %q: %s", want, log)
		}
	}
}
