package toolprocess_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/toolprocess"
)

func TestCommandContextHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cmd := toolprocess.CommandContext(ctx, os.Args[0], "-test.run=^$")
	if err := cmd.Run(); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled command: got %v, want context.Canceled", err)
	}
}
