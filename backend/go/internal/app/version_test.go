package app_test

import (
	"errors"
	"net/url"
	"testing"

	"github.com/Clementine33/preen/backend/internal/app"
	"github.com/Clementine33/preen/backend/internal/updatecheck"
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

func TestUpdateCheckSkipsDevelopmentVersionsWithoutARequest(t *testing.T) {
	t.Parallel()
	resolved := false
	proxy := func(*url.URL) (*url.URL, error) { resolved = true; return nil, errors.New("must not be asked") }
	for _, version := range []string{"0.2.0-dev", "0.2.1-dev+abc123", "dev"} {
		svc := app.NewUpdateCheck(version, proxy)
		svc.Run(t.Context())
		if got := svc.Status().State; got != updatecheck.StateNone {
			t.Errorf("version %s: state = %q", version, got)
		}
	}
	if resolved {
		t.Fatal("a skipped check reached the Outbound Proxy")
	}
}
