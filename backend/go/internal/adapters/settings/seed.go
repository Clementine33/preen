package settings

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed config.json.template
var initialConfig []byte

// SeedDefaults delivers the initial configuration without replacing user files.
// Readers remain independent of this startup step and never fall back to it.
func SeedDefaults(configDir string) error {
	if configDir == "" {
		return nil
	}
	target := filepath.Join(configDir, "config.json")
	if _, err := os.Lstat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check initial configuration: %w", err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	// CreateTemp uses O_CREATE|O_EXCL; publish only a closed, complete file so
	// concurrent first starts cannot observe an unfinished configuration.
	file, err := os.CreateTemp(configDir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create initial configuration: %w", err)
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(initialConfig)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write initial configuration: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close initial configuration: %w", closeErr)
	}
	// A hard link atomically creates the name and refuses any existing target.
	if err := os.Link(file.Name(), target); err != nil && !os.IsExist(err) {
		return fmt.Errorf("publish initial configuration: %w", err)
	}
	return nil
}
