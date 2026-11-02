package supervisor_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Clementine33/preen/desktop/supervisor"
)

func TestResolveLayouts(t *testing.T) {
	for _, tc := range []struct{ platform, executable, home, xdg, service, web, data, path string }{
		{"windows", "/portable/Preen.exe", "/home/person", "", "/portable/bin/preen.exe", "/portable/web", "/portable/data", "/portable/bin;system"},
		{"windows", `C:\Apps\Preen\preen-desktop.exe`, "/home/person", "", "C:/Apps/Preen/bin/preen.exe", "C:/Apps/Preen/web", "C:/Apps/Preen/data", "C:/Apps/Preen/bin;system"},
		{"linux", "/usr/bin/preen-desktop", "/home/person", "", "/usr/lib/preen/preen", "/usr/share/preen/web", "/home/person/.local/share/preen", "system"},
		{"linux", "/usr/bin/preen-desktop", "/home/person", "/custom", "/usr/lib/preen/preen", "/usr/share/preen/web", "/custom/preen", "system"},
	} {
		t.Run(tc.platform+tc.xdg, func(t *testing.T) {
			plan, err := supervisor.Resolve(supervisor.Input{Options: supervisor.Options{GOOS: tc.platform, Executable: tc.executable, HomeDir: tc.home, Environ: []string{"PATH=system", "XDG_DATA_HOME=" + tc.xdg, "PREEN_CORS_ORIGINS=https://external", "PREEN_DATA_DIR=ignored"}, Origin: "wails://wails.localhost", Version: "0.2.0"}, LocalToken: "random"})
			if err != nil {
				t.Fatal(err)
			}
			if plan.Executable != tc.service || plan.DataDir != tc.data || plan.WebDir != tc.web {
				t.Fatalf("layout: %+v", plan)
			}
			if want := []string{"--watch-stdin", "--web-dir=" + tc.web}; !reflect.DeepEqual(plan.Args, want) {
				t.Fatalf("args: %v, want %v", plan.Args, want)
			}
			for key, want := range map[string]string{"PATH": tc.path, "PREEN_TOKEN": "random", "PREEN_DATA_DIR": tc.data, "PREEN_CORS_ORIGINS": "https://external,wails://wails.localhost"} {
				if got := envValue(plan.Env, key); got != want {
					t.Fatalf("%s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestLayoutOf(t *testing.T) {
	for _, tc := range []struct {
		platform, executable string
		want                 supervisor.Layout
	}{
		{"windows", `C:\Apps\Preen\preen-desktop.exe`, supervisor.Layout{Service: "C:/Apps/Preen/bin/preen.exe", WebDir: "C:/Apps/Preen/web", PortableData: "C:/Apps/Preen/data"}},
		{"linux", "/usr/bin/preen-desktop", supervisor.Layout{Service: "/usr/lib/preen/preen", WebDir: "/usr/share/preen/web"}},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			got, err := supervisor.LayoutOf(tc.executable, tc.platform)
			if err != nil || got != tc.want {
				t.Fatalf("layout = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
	if _, err := supervisor.LayoutOf("/Applications/preen", "darwin"); err == nil {
		t.Fatal("unsupported platform must fail")
	}
}

func TestResolveDevServerOmitsWebDir(t *testing.T) {
	for _, platform := range []string{"windows", "linux"} {
		plan, err := supervisor.Resolve(supervisor.Input{Options: supervisor.Options{GOOS: platform, Executable: "/usr/bin/preen-desktop", HomeDir: "/home/person", DevServer: true}, LocalToken: "random"})
		if err != nil {
			t.Fatal(err)
		}
		if plan.WebDir != "" || !reflect.DeepEqual(plan.Args, []string{"--watch-stdin"}) {
			t.Fatalf("%s dev plan serves a web directory: %+v", platform, plan)
		}
	}
}

func TestResolveCredentials(t *testing.T) {
	for _, tc := range []struct{ name, env, config, want string }{
		{"environment wins", "environment", `{"server":{"token":"file"}}`, "environment"},
		{"file token", "", `{"server":{"token":"file"}}`, "file"},
		{"local", "", `{"server":{}}`, "random"},
		{"missing", "", "", "random"},
		{"malformed", "", `{"server":{"allow_lan":true},`, "random"},
		{"lan missing token", "", `{"server":{"allow_lan":true,"port":8765}}`, ""},
		{"lan environment", "environment", `{"server":{"allow_lan":true}}`, "environment"},
		{"unrelated field types", "", `{"server":{"port":"invalid","token":"file"},"prune":false}`, "file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := supervisor.Resolve(supervisor.Input{Options: supervisor.Options{Executable: "/usr/bin/preen-desktop", GOOS: "linux", HomeDir: "/home/person", Environ: []string{"PREEN_TOKEN=" + tc.env}}, Config: []byte(tc.config), LocalToken: "random"})
			if err != nil {
				t.Fatal(err)
			}
			if p.Token != tc.want || envValue(p.Env, "PREEN_TOKEN") != tc.want {
				t.Fatalf("token mismatch: %q", p.Token)
			}
		})
	}
}

func TestPrepareFreshTokenAndUnwritableData(t *testing.T) {
	root := t.TempDir()
	opts := supervisor.Options{Executable: filepath.Join(root, "Preen.exe"), GOOS: "windows", Version: "0.2.0"}
	writeWebFixture(t, opts)
	a, err := supervisor.Prepare(opts)
	if err != nil {
		t.Fatal(err)
	}
	b, err := supervisor.Prepare(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Token) != 64 || a.Token == b.Token {
		t.Fatal("local token must contain fresh 32-byte entropy")
	}
	if err := os.Remove(a.DataDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.DataDir, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := supervisor.Prepare(opts); err == nil {
		t.Fatal("must fail instead of choosing another data directory")
	}
}

func TestPrepareRequiresWebIndexUnlessDevServer(t *testing.T) {
	root := t.TempDir()
	opts := supervisor.Options{Executable: filepath.Join(root, "Preen.exe"), GOOS: "windows", Version: "0.2.0"}
	layout, err := supervisor.LayoutOf(opts.Executable, opts.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = supervisor.Prepare(opts); err == nil || !strings.Contains(err.Error(), "web directory") || !strings.Contains(err.Error(), "index.html") {
		t.Fatalf("missing web directory: %v", err)
	}
	// A directory named index.html is not a readable regular file.
	if err = os.MkdirAll(filepath.Join(layout.WebDir, "index.html"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = supervisor.Prepare(opts); err == nil || !strings.Contains(err.Error(), "web directory") {
		t.Fatalf("index.html directory: %v", err)
	}
	if _, err = os.Stat(layout.PortableData); err == nil {
		t.Fatal("web check must precede data directory creation")
	}
	dev := opts
	dev.DevServer = true
	plan, err := supervisor.Prepare(dev)
	if err != nil || plan.WebDir != "" {
		t.Fatalf("dev server needs no web directory: %+v, %v", plan, err)
	}
}

// writeWebFixture creates the packaged web directory's index.html that Prepare requires.
func writeWebFixture(t *testing.T, opts supervisor.Options) {
	t.Helper()
	layout, err := supervisor.LayoutOf(opts.Executable, opts.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(layout.WebDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(layout.WebDir, "index.html"), []byte("<html></html>"), 0600); err != nil {
		t.Fatal(err)
	}
}

func envValue(env []string, key string) string {
	for _, entry := range env {
		if len(entry) > len(key) && entry[:len(key)+1] == key+"=" {
			return entry[len(key)+1:]
		}
	}
	return ""
}
