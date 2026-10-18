package execute_test

import (
	"context"
	"errors"
	"os"

	"github.com/Clementine33/preen/backend/internal/conversion/execute"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
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
	source, encodeErr := encoder.Encode(ctx, input, output, spec)
	closeErr := output.Close()
	if joinedErr := errors.Join(encodeErr, closeErr); joinedErr != nil {
		return joinedErr
	}
	reader, err := os.Open(dst)
	if err != nil {
		return err
	}
	defer reader.Close()
	_, err = encoder.ValidateOutput(ctx, reader, spec, source)
	return err
}
