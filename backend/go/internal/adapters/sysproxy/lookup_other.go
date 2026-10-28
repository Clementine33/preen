//go:build !windows

package sysproxy

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/toolprocess"
)

// errAutoConfig fails a request rather than guessing a route.
//
// shortcut: a desktop set to PAC or auto-detection is refused on Linux, since
// following it needs a PAC evaluator; add one if such setups are reported.
var errAutoConfig = errors.New(
	"the system proxy uses automatic configuration, which is not supported on this platform; set the proxy manually",
)

// Lookup answers from the desktop's own settings when the session names a
// desktop (KDE: kioslaverc; others: GNOME gsettings), and otherwise from
// HTTP(S)_PROXY / NO_PROXY, as a headless server or container would set them.
func Lookup(target *url.URL) (*url.URL, error) {
	desktop := strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP"))
	switch {
	case loopback(target.Hostname()):
		return nil, nil
	case strings.Contains(desktop, "KDE"):
		if content, err := os.ReadFile(kioslaverc()); err == nil {
			switch proxyType, f := parseKioslaverc(string(content)); proxyType {
			case "", "0":
				return nil, nil
			case "1":
				return f.route(target)
			case "2", "3":
				return nil, errAutoConfig
			}
		}
	case desktop != "":
		if out, err := gsettings(); err == nil {
			switch mode, f := parseGSettings(out); mode {
			case "none":
				return nil, nil
			case "manual":
				return f.route(target)
			case "auto":
				return nil, errAutoConfig
			}
		}
	}
	return http.ProxyFromEnvironment(&http.Request{URL: target})
}

func kioslaverc() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "kioslaverc")
}

// gsettings output is reused for a moment: one metadata batch issues many
// requests, and settings change far less often than that.
var gnome struct {
	mu  sync.Mutex
	at  time.Time
	out string
	err error
}

func gsettings() (string, error) {
	gnome.mu.Lock()
	defer gnome.mu.Unlock()
	if time.Since(gnome.at) > 2*time.Second {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		out, err := toolprocess.CommandContext(ctx, "gsettings", "list-recursively", "org.gnome.system.proxy").Output()
		gnome.at, gnome.out, gnome.err = time.Now(), string(out), err
	}
	return gnome.out, gnome.err
}
