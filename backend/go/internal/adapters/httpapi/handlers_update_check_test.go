package httpapi //nolint:testpackage // white-box tests drive the routes through the real router

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/updatecheck"
)

func updateCheckBody(t *testing.T, svc *updatecheck.Service, token string) (int, string) {
	t.Helper()
	engine := newTestServer(t, func(d *Dependencies, _ *sqlite.Repository) {
		d.UpdateCheck = svc
		d.Token = token
	})
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	w := doRequest(t, engine, http.MethodGet, "/api/v1/update-check", nil, headers)
	return w.Code, w.Body.String()
}

func release(tag string) updatecheck.Source {
	return func(context.Context) (updatecheck.Release, error) {
		return updatecheck.Release{Tag: tag, URL: "https://example.test/releases/" + tag}, nil
	}
}

func TestUpdateCheckEndpointReportsEachState(t *testing.T) {
	available := updatecheck.New("0.2.0", release("v0.3.0"))
	none := updatecheck.New("0.3.0", release("v0.3.0"))
	checking := updatecheck.New("0.2.0", release("v0.3.0"))
	failed := updatecheck.New("0.2.0", func(context.Context) (updatecheck.Release, error) {
		return updatecheck.Release{}, errors.New("status 404")
	})
	for _, svc := range []*updatecheck.Service{available, none, failed} {
		svc.Run(context.Background())
	}
	for _, c := range []struct {
		name string
		svc  *updatecheck.Service
		want string
	}{
		{"available", available, `{"state":"available","latest_version":"v0.3.0","release_url":"https://example.test/releases/v0.3.0"}`},
		{"up to date", none, `{"state":"none"}`},
		{"failed", failed, `{"state":"none"}`},
		{"checking", checking, `{"state":"checking"}`},
		{"not wired", nil, `{"state":"none"}`},
	} {
		if code, got := updateCheckBody(t, c.svc, ""); code != http.StatusOK || got != c.want {
			t.Errorf("%s: %d %s, want %s", c.name, code, got, c.want)
		}
	}
}

func TestUpdateCheckEndpointRequiresAuthentication(t *testing.T) {
	svc := updatecheck.New(
		"0.2.0",
		release("v0.3.0"),
		updatecheck.WithSleep(func(context.Context, time.Duration) error { return nil }),
	)
	engine := newTestServer(t, func(d *Dependencies, _ *sqlite.Repository) {
		d.UpdateCheck = svc
		d.Token = "secret"
	})
	if w := doRequest(t, engine, http.MethodGet, "/api/v1/update-check", nil, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d %s", w.Code, w.Body.String())
	}
	if code, _ := updateCheckBody(t, svc, "secret"); code != http.StatusOK {
		t.Fatalf("authenticated = %d", code)
	}
}
