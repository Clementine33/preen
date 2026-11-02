package supervisor

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Prepare checks the packaged web directory (unless fronting a dev server),
// probes the one data directory, reads config.json, and supplies fresh local
// credentials. It never writes configuration or falls back to other data.
func Prepare(options Options) (Plan, error) {
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return Plan{}, fmt.Errorf("generate service token: %w", err)
	}
	in := Input{Options: options, LocalToken: hex.EncodeToString(entropy)}
	plan, err := Resolve(in)
	if err != nil {
		return Plan{}, err
	}
	if plan.WebDir != "" {
		if err = checkWebDir(plan.WebDir); err != nil {
			return Plan{}, err
		}
	}
	if err = os.MkdirAll(plan.DataDir, 0700); err != nil {
		return Plan{}, fmt.Errorf("data directory %s is not writable: %w", plan.DataDir, err)
	}
	probe, err := os.CreateTemp(plan.DataDir, ".preen-write-*")
	if err != nil {
		return Plan{}, fmt.Errorf("data directory %s is not writable: %w", plan.DataDir, err)
	}
	closeErr := probe.Close()
	removeErr := os.Remove(probe.Name())
	if closeErr != nil {
		return Plan{}, fmt.Errorf("probe data directory: %w", closeErr)
	}
	if removeErr != nil {
		return Plan{}, fmt.Errorf("remove data directory probe: %w", removeErr)
	}
	// An unreadable/malformed config is still passed to the service to diagnose.
	in.Config, _ = os.ReadFile(filepath.Join(plan.DataDir, "config.json"))
	return Resolve(in)
}

// checkWebDir requires a readable index.html so a broken package fails with a
// clear reason instead of the service serving nothing.
func checkWebDir(dir string) error {
	index, err := os.Open(filepath.Join(dir, "index.html"))
	if err != nil {
		return fmt.Errorf("web directory %s must contain index.html: %w", dir, err)
	}
	defer func() { _ = index.Close() }()
	info, err := index.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("not a regular file")
	}
	if err != nil {
		return fmt.Errorf("web directory %s must contain index.html: %w", dir, err)
	}
	return nil
}
