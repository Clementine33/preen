package wirecheck

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

var observed sync.Map

// WriteCoverage records actual HTTP/status and SSE/event coverage for a whole
// package run. Set PREEN_WIRE_COVERAGE to an output path for an acceptance audit.
func WriteCoverage() error {
	filename := os.Getenv("PREEN_WIRE_COVERAGE")
	require := os.Getenv("PREEN_WIRE_REQUIRE_COVERAGE") == "1"
	if filename == "" && !require {
		return nil
	}
	seen := []string{}
	observed.Range(func(key, value any) bool {
		if s, ok := key.(string); ok {
			seen = append(seen, s)
		}
		return true
	})
	sort.Strings(seen)
	missing := []string{}
	v := cached()
	if v.err != nil {
		return v.err
	}
	for pattern, c := range v.contracts.HTTP {
		for status, name := range c.Responses {
			if strings.HasPrefix(name, "@") && name != "@sse" {
				continue
			}
			key := pattern + " " + status
			if _, ok := observed.Load(key); !ok {
				missing = append(missing, key)
			}
		}
	}
	for family, events := range v.contracts.SSE {
		for event := range events {
			key := "SSE " + family + " " + event
			if _, ok := observed.Load(key); !ok {
				missing = append(missing, key)
			}
		}
	}
	sort.Strings(missing)
	raw, err := json.MarshalIndent(map[string]any{"observed": seen, "missing": missing}, "", "  ")
	if err != nil {
		return err
	}
	if filename != "" {
		if err = os.WriteFile(filename, append(raw, '\n'), 0o644); err != nil {
			return err
		}
	}
	if require && len(missing) > 0 {
		return fmt.Errorf("missing wire contract coverage:\n%s", strings.Join(missing, "\n"))
	}
	return nil
}
