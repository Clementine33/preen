package app_test

import (
	"testing"

	"github.com/Clementine33/preen/backend/internal/app"
)

func TestDatabaseModeFollowsDevelopmentPrerelease(t *testing.T) {
	t.Parallel()
	for version, development := range map[string]bool{
		"0.2.0-dev":        true,
		"0.2.1-dev+abc123": true,
		"0.2.1-dev.2":      true,
		"0.2.0":            false,
		"0.2.1":            false,
		"0.2.1-rc.1":       false,
		"0.2.1+dev":        false,
		"0.2.1-device":     false,
	} {
		if got := app.IsDevelopmentVersion(version); got != development {
			t.Errorf("version %s development = %t, want %t", version, got, development)
		}
	}
}
