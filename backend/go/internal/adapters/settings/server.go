package settings

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

// ServerConfig is read once at startup. Its zero value preserves the local
// developer listener; LAN access requires a fixed port and a shared bearer key.
type ServerConfig struct {
	AllowLAN bool   `json:"allow_lan"`
	Port     int    `json:"port"`
	Token    string `json:"token"`
}

// ListenAddress names the listener, never a client-facing connection address.
func (s ServerConfig) ListenAddress() string {
	host := "127.0.0.1"
	if s.AllowLAN {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, strconv.Itoa(s.Port))
}

// Server loads and validates startup settings. A nonempty host-provided token
// overrides the file key; non-nil listener overrides replace their file fields
// before validation. An unreadable or invalid file fails startup rather
// than silently replacing the requested access policy with defaults.
func (r Reader) Server(tokenOverride string, allowLAN *bool, port *int) (ServerConfig, error) {
	cfg := DefaultAppConfig()
	if r.configDir != "" {
		data, err := os.ReadFile(filepath.Join(r.configDir, "config.json"))
		if err != nil && !os.IsNotExist(err) {
			return ServerConfig{}, fmt.Errorf("read server configuration: %w", err)
		}
		if err == nil {
			if err := json.Unmarshal(data, &cfg); err != nil {
				return ServerConfig{}, fmt.Errorf("decode server configuration: %w", err)
			}
		}
	}
	server := cfg.Server
	if allowLAN != nil {
		server.AllowLAN = *allowLAN
	}
	if port != nil {
		server.Port = *port
	}
	if tokenOverride != "" {
		server.Token = tokenOverride
	}
	if server.Port < 0 || server.Port > 65535 {
		return ServerConfig{}, fmt.Errorf("server.port must be between 0 and 65535")
	}
	for _, char := range server.Token {
		if char < 0x21 || char > 0x7e {
			return ServerConfig{}, fmt.Errorf("server token must contain only visible ASCII characters without spaces")
		}
	}
	if server.AllowLAN && (server.Port == 0 || server.Token == "") {
		return ServerConfig{}, fmt.Errorf("server.allow_lan requires a fixed nonzero port and a nonempty token")
	}
	return server, nil
}
