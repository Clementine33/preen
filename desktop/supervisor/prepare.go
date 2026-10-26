package supervisor

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// Prepare probes the one data directory, reads config.json, and supplies fresh
// local credentials. It never writes configuration or falls back to other data.
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
