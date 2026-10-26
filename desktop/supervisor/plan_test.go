package supervisor_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Clementine33/preen/desktop/supervisor"
)

func TestResolveLayouts(t *testing.T) {
	for _, tc := range []struct{ platform, executable, home, xdg, service, data, path string }{
		{"windows", "/portable/Preen.exe", "/home/person", "", "/portable/bin/preen.exe", "/portable/data", "/portable/bin;system"},
		{"linux", "/usr/bin/preen-desktop", "/home/person", "", "/usr/lib/preen/preen", "/home/person/.local/share/preen", "system"},
		{"linux", "/usr/bin/preen-desktop", "/home/person", "/custom", "/usr/lib/preen/preen", "/custom/preen", "system"},
	} {
		t.Run(tc.platform+tc.xdg, func(t *testing.T) {
			plan, err := supervisor.Resolve(supervisor.Input{Options: supervisor.Options{GOOS: tc.platform, Executable: tc.executable, HomeDir: tc.home, Environ: []string{"PATH=system", "XDG_DATA_HOME=" + tc.xdg, "PREEN_CORS_ORIGINS=https://external", "PREEN_DATA_DIR=ignored"}, Origin: "wails://wails.localhost", Version: "0.2.0"}, LocalToken: "random"})
			if err != nil {
				t.Fatal(err)
			}
			if plan.Executable != tc.service || plan.DataDir != tc.data {
				t.Fatalf("layout: %+v", plan)
			}
			if !reflect.DeepEqual(plan.Args, []string{"--watch-stdin"}) {
				t.Fatalf("args: %v", plan.Args)
			}
			for key, want := range map[string]string{"PATH": tc.path, "PREEN_TOKEN": "random", "PREEN_DATA_DIR": tc.data, "PREEN_CORS_ORIGINS": "https://external,wails://wails.localhost"} {
				if got := envValue(plan.Env, key); got != want {
					t.Fatalf("%s = %q, want %q", key, got, want)
				}
			}
		})
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

func envValue(env []string, key string) string {
	for _, entry := range env {
		if len(entry) > len(key) && entry[:len(key)+1] == key+"=" {
			return entry[len(key)+1:]
		}
	}
	return ""
}
