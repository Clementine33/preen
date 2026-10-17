package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/settings"
)

func TestServerSettings(t *testing.T) {
	for _, tc := range []struct {
		name, config, override, address, token string
		invalid                                bool
	}{
		{name: "missing file", address: "127.0.0.1:0"},
		{name: "legacy file", config: `{"prune":{"no_effect_tags":[]}}`, address: "127.0.0.1:0"},
		{name: "local fixed port", config: `{"server":{"port":8080}}`, address: "127.0.0.1:8080"},
		{name: "LAN", config: `{"server":{"allow_lan":true,"port":8080,"token":"file-key"}}`, address: "0.0.0.0:8080", token: "file-key"},
		{name: "host key overrides file", config: `{"server":{"allow_lan":true,"port":65535,"token":"file-key"}}`, override: "host-key", address: "0.0.0.0:65535", token: "host-key"},
		{name: "host key enables LAN", config: `{"server":{"allow_lan":true,"port":8080}}`, override: "host-key", address: "0.0.0.0:8080", token: "host-key"},
		{name: "missing LAN key", config: `{"server":{"allow_lan":true,"port":8080}}`, invalid: true},
		{name: "random LAN port", config: `{"server":{"allow_lan":true,"token":"file-key"}}`, invalid: true},
		{name: "negative port", config: `{"server":{"port":-1}}`, invalid: true},
		{name: "port overflow", config: `{"server":{"port":65536}}`, invalid: true},
		{name: "wrong port type", config: `{"server":{"port":"8080"}}`, invalid: true},
		{name: "spaces in key", config: `{"server":{"token":"two words"}}`, invalid: true},
		{name: "newline in key", config: `{"server":{"token":"key\nline"}}`, invalid: true},
		{name: "non ASCII key", config: `{"server":{"token":"密钥"}}`, invalid: true},
		{name: "invalid override", override: "host key", invalid: true},
		{name: "invalid JSON", config: `{bad json`, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.config != "" {
				if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(tc.config), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := settings.NewReader(dir).Server(tc.override)
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid server settings accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ListenAddress() != tc.address || got.Token != tc.token {
				t.Fatalf("got address %q and key %q, want %q and %q",
					got.ListenAddress(), got.Token, tc.address, tc.token)
			}
		})
	}
}

func TestServerRejectsUnreadableConfiguration(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "config.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.NewReader(dir).Server(""); err == nil {
		t.Fatal("configuration read failure silently ignored")
	}
}
