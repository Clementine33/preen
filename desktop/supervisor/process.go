package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// API is the exact connection value returned to the desktop page.
type API struct {
	BaseURL string `json:"baseUrl"`
	Token   string `json:"token"`
}

// UnexpectedExit includes all recently captured stdout and stderr, including
// diagnostics emitted before readiness. A clean unsolicited exit is unexpected.
type UnexpectedExit struct {
	Cause error
	Tail  string
}

func (e *UnexpectedExit) Error() string {
	return fmt.Sprintf("Preen service exited unexpectedly: %v\n%s", e.Cause, e.Tail)
}
func (e *UnexpectedExit) Unwrap() error { return e.Cause }

// Process owns one child. ResolveAPI waits for readiness; Wait reports its end;
// Stop is idempotent and waits at most ten seconds before killing the child.
type Process struct {
	cmd                       *exec.Cmd
	stdin                     io.WriteCloser
	output                    *output
	version, token            string
	ready, done               chan struct{}
	readyOnce, stopOnce       sync.Once
	mu                        sync.Mutex
	api                       API
	readyErr, result, stopErr error
	tail                      string
	stopping                  bool
}

// Start starts collecting both output streams before the service runs. The
// context owns its lifetime; cancellation also stops a child awaiting readiness.
func Start(ctx context.Context, plan Plan) (*Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out, err := openOutput(plan.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open service log: %w", err)
	}
	cmd := exec.Command(plan.Executable, plan.Args...)
	cmd.Env = plan.Env
	cmd.WaitDelay = time.Second
	configureProcess(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_, _ = out.finish()
		return nil, err
	}
	p := &Process{cmd: cmd, stdin: stdin, output: out, version: plan.Version, token: plan.Token, ready: make(chan struct{}), done: make(chan struct{})}
	cmd.Stdout = &readinessWriter{process: p}
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		tail, logErr := out.finish()
		return nil, &UnexpectedExit{Cause: errors.Join(err, logErr), Tail: tail}
	}
	go p.wait()
	go func() {
		select {
		case <-ctx.Done():
			_ = p.Stop()
		case <-p.done:
		}
	}()
	return p, nil
}

// ResolveAPI waits for a matching-version handshake. Cancellation releases the
// process before returning, so a canceled startup cannot leave a service behind.
func (p *Process) ResolveAPI(ctx context.Context) (API, error) {
	select {
	case <-ctx.Done():
		_ = p.Stop()
		return API{}, ctx.Err()
	case <-p.ready:
	}
	p.mu.Lock()
	api, readyErr, result := p.api, p.readyErr, p.result
	p.mu.Unlock()
	if readyErr != nil {
		_ = p.Stop()
		p.mu.Lock()
		tail := p.tail
		p.mu.Unlock()
		return API{}, &UnexpectedExit{Cause: readyErr, Tail: tail}
	}
	if api.BaseURL == "" {
		if result == nil {
			result = errors.New("Preen service stopped before readiness")
		}
		return API{}, result
	}
	return api, nil
}

// Wait returns nil for a requested stop and UnexpectedExit otherwise.
func (p *Process) Wait() error {
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.result
}

// Stop closes the stdin shutdown seam and kills a service still alive after
// ten seconds. It returns an error when a forced termination was necessary.
func (p *Process) Stop() error {
	p.stopOnce.Do(func() {
		p.mu.Lock()
		select {
		case <-p.done:
			p.mu.Unlock()
			return
		default:
		}
		p.stopping = true
		p.mu.Unlock()
		_ = p.stdin.Close()
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-p.done:
		case <-timer.C:
			p.stopErr = errors.Join(errors.New("Preen service did not stop within 10 seconds; forced termination"), p.cmd.Process.Kill())
			<-p.done
		}
	})
	return p.stopErr
}

func (p *Process) wait() {
	err := p.cmd.Wait()
	_ = p.stdin.Close()
	tail, logErr := p.output.finish()
	p.mu.Lock()
	p.tail = tail
	if !p.stopping {
		if err == nil {
			err = errors.New("process ended without a stop request")
		}
		p.result = &UnexpectedExit{Cause: errors.Join(err, logErr), Tail: tail}
	} else {
		p.result = logErr
	}
	close(p.done)
	p.mu.Unlock()
	p.readyOnce.Do(func() { close(p.ready) })
}

type readinessWriter struct {
	process  *Process
	line     []byte
	overflow bool
}

func (w *readinessWriter) Write(data []byte) (int, error) {
	n, err := w.process.output.Write(data)
	for _, b := range data {
		if b == '\n' {
			if !w.overflow {
				w.process.handshake(strings.TrimSuffix(string(w.line), "\r"))
			}
			w.line = w.line[:0]
			w.overflow = false
			continue
		}
		if len(w.line) < 4096 {
			w.line = append(w.line, b)
		} else {
			w.overflow = true
		}
	}
	return n, err
}

func (p *Process) handshake(line string) {
	fields := strings.Fields(line)
	if len(fields) != 3 || fields[0] != "PREEN_BACKEND_READY" || !strings.HasPrefix(fields[1], "version=") || !strings.HasPrefix(fields[2], "http_port=") {
		return
	}
	port, err := strconv.Atoi(strings.TrimPrefix(fields[2], "http_port="))
	if err != nil || port < 1 || port > 65535 {
		return
	}
	p.readyOnce.Do(func() {
		p.mu.Lock()
		version := strings.TrimPrefix(fields[1], "version=")
		if version != p.version {
			p.readyErr = fmt.Errorf("Preen service version %q does not match Desktop Shell version %q", version, p.version)
		} else {
			p.api = API{BaseURL: fmt.Sprintf("http://127.0.0.1:%d/api/v1", port), Token: p.token}
		}
		close(p.ready)
		mismatch := p.readyErr != nil
		p.mu.Unlock()
		if mismatch {
			go func() { _ = p.Stop() }()
		}
	})
}
