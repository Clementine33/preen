package app_test

import (
	"testing"

	"github.com/Clementine33/preen/backend/internal/app"
)

func TestBuildHandshakeLine(t *testing.T) {
	t.Parallel()
	got := app.BuildHandshakeLine("v1", 54321)
	want := "PREEN_BACKEND_READY version=v1 http_port=54321"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
