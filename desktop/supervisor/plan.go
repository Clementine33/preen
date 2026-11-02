// Package supervisor resolves the Desktop Shell's service launch plan and owns
// its child process, readiness, diagnostic logs and bounded shutdown. It uses no
// backend or window-framework code; Resolve is the supplied-fact planning seam,
// Prepare reads the deployment, and Start manages one service lifetime.
package supervisor

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// Options supplies the shell location and inherited environment explicitly.
// DevServer marks a shell fronting a Vite dev server: the service then serves
// no web directory and the packaged one is not required.
type Options struct {
	Executable string
	GOOS       string
	Environ    []string
	HomeDir    string
	Origin     string
	Version    string
	DevServer  bool
}

// Layout is the packaged program layout derived from the shell executable.
// Paths use forward slashes, which both supported platforms accept.
type Layout struct {
	// Service is the supervised backend executable.
	Service string
	// WebDir is the one web build directory that the shell and the service share.
	WebDir string
	// PortableData is the data directory beside a portable Windows install;
	// it is empty on Linux, where data follows the XDG base directory.
	PortableData string
}

// LayoutOf computes the packaged layout from the shell executable path and
// GOOS without touching the filesystem.
func LayoutOf(executable, goos string) (Layout, error) {
	dir := path.Dir(strings.ReplaceAll(executable, "\\", "/"))
	switch goos {
	case "windows":
		return Layout{Service: path.Join(dir, "bin/preen.exe"), WebDir: path.Join(dir, "web"), PortableData: path.Join(dir, "data")}, nil
	case "linux":
		return Layout{Service: path.Join(dir, "../lib/preen/preen"), WebDir: path.Join(dir, "../share/preen/web")}, nil
	default:
		return Layout{}, fmt.Errorf("unsupported desktop platform %q", goos)
	}
}

// Input adds the configuration bytes and a fresh local-only token to Options.
// Supplying these facts keeps Resolve free of filesystem and random I/O.
type Input struct {
	Options
	Config     []byte
	LocalToken string
}

// Plan contains only the facts needed to launch and connect to one service.
type Plan struct {
	Executable string
	Args       []string
	Env        []string
	DataDir    string
	WebDir     string // the web build the service serves; empty in dev-server mode
	Token      string
	Version    string
}

// Resolve computes the fixed layout and effective credentials. Invalid config
// falls back only for token selection; the service still diagnoses the file.
func Resolve(in Input) (Plan, error) {
	env := append([]string(nil), in.Environ...)
	get := func(key string) string {
		for i := len(env) - 1; i >= 0; i-- {
			k, v, ok := strings.Cut(env[i], "=")
			if ok && sameKey(k, key, in.GOOS) {
				return v
			}
		}
		return ""
	}
	set := func(key, value string) {
		out := env[:0]
		for _, entry := range env {
			k, _, _ := strings.Cut(entry, "=")
			if !sameKey(k, key, in.GOOS) {
				out = append(out, entry)
			}
		}
		env = append(out, key+"="+value)
	}
	layout, err := LayoutOf(in.Executable, in.GOOS)
	if err != nil {
		return Plan{}, err
	}
	service, data := layout.Service, layout.PortableData
	switch in.GOOS {
	case "windows":
		bin := path.Dir(service)
		if old := get("PATH"); old != "" {
			bin += ";" + old
		}
		set("PATH", bin)
	case "linux":
		base := get("XDG_DATA_HOME")
		if base == "" {
			base = path.Join(in.HomeDir, ".local/share")
		}
		data = path.Join(base, "preen")
	}
	var config struct {
		Server struct {
			Token    string `json:"token"`
			AllowLAN bool   `json:"allow_lan"`
		} `json:"server"`
	}
	if json.Unmarshal(in.Config, &config) != nil {
		config.Server.Token = ""
		config.Server.AllowLAN = false
	}
	token := get("PREEN_TOKEN")
	if token == "" {
		token = config.Server.Token
	}
	if token == "" && !config.Server.AllowLAN {
		token = in.LocalToken
	}
	set("PREEN_DATA_DIR", data)
	set("PREEN_TOKEN", token)
	if in.Origin != "" {
		cors := get("PREEN_CORS_ORIGINS")
		if cors != "" {
			cors += ","
		}
		set("PREEN_CORS_ORIGINS", cors+in.Origin)
	}
	args := []string{"--watch-stdin"}
	var webDir string
	if !in.DevServer {
		webDir = layout.WebDir
		args = append(args, "--web-dir="+webDir)
	}
	return Plan{Executable: service, Args: args, Env: env, DataDir: data, WebDir: webDir, Token: token, Version: in.Version}, nil
}

func sameKey(a, b, platform string) bool {
	return a == b || platform == "windows" && strings.EqualFold(a, b)
}
