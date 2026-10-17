package ffmpeg_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/ffmpeg"
	"github.com/onsei/organizer/backend/internal/conversion/execute"
	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
)

func outputFixture(
	t *testing.T,
	src string,
	spec reconcile.AudioOutputSpec,
) (execute.Encoder, execute.AudioStream, []byte) {
	t.Helper()
	encoder := ffmpeg.New(ffmpeg.ToolsConfig{})
	input, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	path := filepath.Join(t.TempDir(), "output.stage")
	writer, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	source, err := encoder.Encode(t.Context(), input, writer, spec)
	closeErr := writer.Close()
	if err != nil || closeErr != nil {
		t.Fatal(errors.Join(err, closeErr))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	validateBytes(t, encoder, source, spec, data, true)
	return encoder, source, data
}

func validateBytes(
	t *testing.T,
	encoder execute.Encoder,
	source execute.AudioStream,
	spec reconcile.AudioOutputSpec,
	data []byte,
	valid bool,
) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "output.stage")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	result, err := encoder.ValidateOutput(t.Context(), file, spec, source)
	if valid {
		if err != nil {
			t.Fatal(err)
		}
		if !spec.Lossless() {
			sum := sha256.Sum256(data)
			if result.SHA256 != hex.EncodeToString(sum[:]) {
				t.Fatal("digest does not cover the exact output bytes")
			}
		}
	} else if err == nil {
		t.Fatal("damaged output accepted")
	}
}

func TestMP3OutputValidationAcrossFormats(t *testing.T) {
	for _, rate := range []int{8000, 11025, 16000, 22050, 24000, 32000, 44100, 48000} {
		for _, channels := range []int{1, 2} {
			t.Run(fmt.Sprintf("%d/%d", rate, channels), func(t *testing.T) {
				src := filepath.Join(t.TempDir(), "input.wav")
				runFFmpeg(
					t,
					"-f",
					"lavfi",
					"-i",
					fmt.Sprintf("anoisesrc=duration=1:sample_rate=%d:seed=1", rate),
					"-ac",
					strconv.Itoa(channels),
					"-c:a",
					"pcm_s24le",
					src,
				)
				spec := reconcile.AudioOutputSpec{
					Codec:   reconcile.CodecMp3,
					Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 32},
				}
				outputFixture(t, src, spec)
			})
		}
	}
}

func TestValidationCommandRejectsSuccessfulStderrAndPrioritizesCancellation(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := func(ctx context.Context) *exec.Cmd {
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestValidationStderrProcess$")
		cmd.Env = append(os.Environ(), "ONSEI_VALIDATION_STDERR=1")
		return cmd
	}
	if err := ffmpeg.ValidationCommandForTest(
		t.Context(),
		command(t.Context()),
	); err == nil ||
		!strings.Contains(err.Error(), "decoder diagnostic") {
		t.Fatalf("stderr: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := ffmpeg.ValidationCommandForTest(ctx, command(ctx)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestValidationStderrProcess(t *testing.T) {
	if os.Getenv("ONSEI_VALIDATION_STDERR") != "1" {
		return
	}
	fmt.Fprintln(os.Stderr, "decoder diagnostic")
	os.Exit(0)
}

func TestMP3LargeID3FromAdapter(t *testing.T) {
	_, wav := audioFixture(t)
	dir := t.TempDir()
	metadata := filepath.Join(dir, "tags.txt")
	if err := os.WriteFile(
		metadata,
		[]byte(";FFMETADATA1\ntitle="+strings.Repeat("large metadata ", 20000)+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "tagged.flac")
	runFFmpeg(t, "-i", wav, "-f", "ffmetadata", "-i", metadata, "-map_metadata", "1", "-c:a", "flac", source)
	spec := reconcile.AudioOutputSpec{
		Codec:   reconcile.CodecMp3,
		Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 128},
	}
	_, _, data := outputFixture(t, source, spec)
	length := 0
	for _, b := range data[6:10] {
		length = length<<7 | int(b)
	}
	if length < 256*1024 {
		t.Fatalf("fixture ID3 size=%d", length)
	}
}
