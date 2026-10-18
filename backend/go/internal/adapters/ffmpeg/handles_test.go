package ffmpeg_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/ffmpeg"
	"github.com/Clementine33/preen/backend/internal/conversion/execute"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
)

// Test fixtures own path creation; the encoder only sees handles.
func encodePaths(encoder execute.Encoder, ctx context.Context, src, dst string, spec reconcile.AudioOutputSpec) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
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

// M4A is not an eligible lossless conversion source. Its ordinary AAC stream
// still tests input seeking through the same FD probe and full decode methods.
func TestFDInputSeekableM4AMoovAtEnd(t *testing.T) {
	_, wav := audioFixture(t)
	src := filepath.Join(t.TempDir(), "source.m4a")
	runFFmpeg(t, "-i", wav, "-metadata", "title=seekable input", "-c:a", "aac", src)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Index(data, []byte("moov")) <= bytes.Index(data, []byte("mdat")) {
		t.Fatal("fixture moov is not after mdat")
	}
	input, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if _, seekErr := input.Seek(5, 0); seekErr != nil {
		t.Fatal(seekErr)
	}
	stream, err := ffmpeg.ProbeHandleForTest(ffmpeg.Encoder{}, t.Context(), input)
	if err != nil || stream.Codec != "aac" {
		t.Fatalf("FD probe = %+v, %v", stream, err)
	}
	if err := ffmpeg.DecodeHandleForTest(ffmpeg.Encoder{}, t.Context(), input); err != nil {
		t.Fatal(err)
	}
}
