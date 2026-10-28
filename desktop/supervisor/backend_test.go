package supervisor_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Clementine33/preen/desktop/supervisor"
)

const releaseVersion = "0.2.0"

var backendBuild struct {
	sync.Once
	root, path string
	err        error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if backendBuild.root != "" {
		_ = os.RemoveAll(backendBuild.root)
	}
	os.Exit(code)
}

func realBackend(t *testing.T) string {
	t.Helper()
	if binary := os.Getenv("PREEN_SUPERVISOR_TEST_BINARY"); binary != "" {
		return binary
	}
	backendBuild.Do(func() {
		backendBuild.root, backendBuild.err = os.MkdirTemp("", "preen-supervisor-backend-*")
		if backendBuild.err != nil {
			return
		}
		backendBuild.path = filepath.Join(backendBuild.root, "preen")
		if runtime.GOOS == "windows" {
			backendBuild.path += ".exe"
		}
		_, file, _, _ := runtime.Caller(0)
		cmd := exec.Command("go", "build", "-ldflags", "-X main.version="+releaseVersion, "-o", backendBuild.path, "./cmd/preen")
		cmd.Dir = filepath.Join(filepath.Dir(file), "../../backend/go")
		if out, err := cmd.CombinedOutput(); err != nil {
			backendBuild.err = errors.New(err.Error() + ": " + string(out))
		}
	})
	if backendBuild.err != nil {
		t.Fatal(backendBuild.err)
	}
	return backendBuild.path
}

func backendPlan(t *testing.T, config string) supervisor.Plan {
	t.Helper()
	root := t.TempDir()
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "PREEN_") && !strings.HasPrefix(entry, "XDG_DATA_HOME=") {
			env = append(env, entry)
		}
	}
	options := supervisor.Options{Executable: filepath.Join(root, "bin", "preen-desktop"), GOOS: runtime.GOOS, Environ: append(env, "XDG_DATA_HOME="+root), HomeDir: root, Origin: "http://wails.localhost", Version: releaseVersion}
	p, err := supervisor.Prepare(options)
	if err != nil {
		t.Fatal(err)
	}
	if config != "" {
		if err = os.WriteFile(filepath.Join(p.DataDir, "config.json"), []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
		p, err = supervisor.Prepare(options)
		if err != nil {
			t.Fatal(err)
		}
	}
	p.Executable = realBackend(t)
	return p
}

func TestRealBackendDefaultAuthenticationAndLogRotation(t *testing.T) {
	plan := backendPlan(t, "")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for attempt := 0; attempt < 3; attempt++ {
		p, err := supervisor.Start(ctx, plan)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = p.Stop() })
		api, err := p.ResolveAPI(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(api.BaseURL, "http://127.0.0.1:") || api.Token != plan.Token || len(api.Token) != 64 {
			t.Fatalf("connection: %+v", api)
		}
		assertBearer(t, api, "", http.StatusUnauthorized)
		assertBearer(t, api, api.Token, http.StatusOK)
		if err = p.Stop(); err != nil {
			t.Fatal(err)
		}
		if err = p.Wait(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(plan.DataDir, "logs", "preen.log"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "PREEN_BACKEND_READY version="+releaseVersion) {
			t.Fatalf("missing handshake: %s", data)
		}
		if attempt > 0 {
			previous, err := os.ReadFile(filepath.Join(plan.DataDir, "logs", "preen.prev.log"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(previous), "PREEN_BACKEND_READY") {
				t.Fatal("rotation lost previous service output")
			}
		}
	}
	entries, err := os.ReadDir(filepath.Join(plan.DataDir, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("retained %d logs", len(entries))
	}
}

func TestRealBackendFixedPortAndKey(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	plan := backendPlan(t, `{"server":{"port":`+strconv.Itoa(port)+`,"token":"configured-key"}}`)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	p, err := supervisor.Start(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop() })
	api, err := p.ResolveAPI(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if api.BaseURL != "http://127.0.0.1:"+strconv.Itoa(port)+"/api/v1" || api.Token != "configured-key" {
		t.Fatalf("fixed configuration lost: %+v", api)
	}
	assertBearer(t, api, "configured-key", http.StatusOK)
	if err = p.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestRealBackendStartupFailuresKeepServiceReason(t *testing.T) {
	for _, tc := range []struct{ name, config, version, reason string }{
		{"LAN without token", `{"server":{"allow_lan":true,"port":8765}}`, releaseVersion, "requires a fixed nonzero port and a nonempty token"},
		{"malformed config", `{"server":`, releaseVersion, "decode server configuration"},
		{"partial upgrade", "", "0.3.0", "does not match"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := backendPlan(t, tc.config)
			plan.Version = tc.version
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			p, err := supervisor.Start(ctx, plan)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = p.Stop() })
			_, err = p.ResolveAPI(ctx)
			var exit *supervisor.UnexpectedExit
			if !errors.As(err, &exit) || !strings.Contains(err.Error(), tc.reason) || exit.Tail == "" {
				t.Fatalf("missing service failure reason: %v", err)
			}
		})
	}
}

func assertBearer(t *testing.T, api supervisor.API, token string, want int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, api.BaseURL+"/libraries", nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != want {
		t.Fatalf("Bearer request status %d, want %d", response.StatusCode, want)
	}
}
