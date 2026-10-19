package e2e //nolint:testpackage // uses the shared backend process harness

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackendWebFlagsAndOverrides(t *testing.T) {
	bin := buildBackendBinary(t)
	webDir := t.TempDir()
	mustWriteFile(t, filepath.Join(webDir, "index.html"), "<html>preen web</html>")
	t.Run("explicit false and zero override file", func(t *testing.T) {
		dataDir := t.TempDir()
		mustWriteFile(
			t,
			filepath.Join(dataDir, "config.json"),
			`{"server":{"allow_lan":true,"port":8765,"token":"file-key"}}`,
		)
		proc := startBackendBinary(t, bin, dataDir, "", "--allow-lan=false", "--port=0", "--web-dir", webDir)
		assertWebResponse(t, proc.httpPort)
	})
	t.Run("LAN flags preserve file key", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address, ok := listener.Addr().(*net.TCPAddr)
		if !ok {
			t.Fatal("unexpected listener address")
		}
		port := address.Port
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
		dataDir := t.TempDir()
		mustWriteFile(t, filepath.Join(dataDir, "config.json"), `{"server":{"token":"file-key"}}`)
		proc := startBackendBinary(
			t,
			bin,
			dataDir,
			"",
			"--allow-lan",
			fmt.Sprintf("--port=%d", port),
			"--web-dir",
			webDir,
		)
		if proc.httpPort != port {
			t.Fatalf("port %d, want %d", proc.httpPort, port)
		}
		assertWebResponse(t, proc.httpPort)
		client := &http.Client{}
		base := fmt.Sprintf("http://127.0.0.1:%d", port)
		if status := doJSON(
			t,
			client,
			t.Context(),
			base,
			"GET",
			"/api/v1/libraries",
			"file-key",
			nil,
			nil,
		); status != 200 {
			t.Fatalf("API status %d", status)
		}
	})
	for _, tc := range []struct {
		name, token, want string
		args              []string
	}{
		{"missing key", "", "requires a fixed nonzero port and a nonempty token", []string{"--allow-lan", "--port=8765"}},
		{"random LAN port", "key", "requires a fixed nonzero port and a nonempty token", []string{"--allow-lan", "--port=0"}},
		{"bad port", "key", "between 0 and 65535", []string{"--port=-1"}},
		{"missing web directory", "", "web directory", []string{"--web-dir", filepath.Join(webDir, "missing")}},
		{"missing index", "", "web directory", []string{"--web-dir", t.TempDir()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			//nolint:gosec // runs the freshly built binary with literal test flags
			cmd := exec.CommandContext(
				t.Context(),
				bin,
				tc.args...)
			cmd.Env = append(os.Environ(), "PREEN_DATA_DIR="+dataDir, "PREEN_TOKEN="+tc.token)
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), tc.want) ||
				strings.Contains(string(output), "PREEN_BACKEND_READY") {
				t.Fatalf("err %v, output %s", err, output)
			}
			if _, err := os.Stat(filepath.Join(dataDir, "cache.db")); !os.IsNotExist(err) {
				t.Fatalf("failed startup created database: %v", err)
			}
		})
	}
}

func assertWebResponse(t *testing.T, port int) {
	t.Helper()
	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		fmt.Sprintf("http://127.0.0.1:%d/works/RJ123456", port),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "<html>preen web</html>" {
		t.Fatalf("status %d, body %s", response.StatusCode, body)
	}
}
