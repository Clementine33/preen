package execute_test

import (
	"context"
	"os"

	"github.com/onsei/organizer/backend/internal/conversion/execute"
	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
)

func encodePaths(encoder execute.Encoder, ctx context.Context, src, dst string, spec reconcile.AudioOutputSpec) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(dst, os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer output.Close()
	_, err = encoder.Encode(ctx, input, output, spec)
	return err
}
