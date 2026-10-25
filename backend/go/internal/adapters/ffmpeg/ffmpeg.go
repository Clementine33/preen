// Package ffmpeg is the media tool adapter behind the execution pipeline: it
// probes streams with ffprobe and writes outputs with ffmpeg. Everything an
// invocation is made of — the arguments, the tool paths, the post-encode
// validation — lives here; the pipeline only names the target specification.
package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Clementine33/preen/backend/internal/conversion/execute"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
)

// Encoder is one configured pair of tools: it encodes the frozen target
// specification independently of global encoder preferences. Empty paths use
// the executables on PATH.
type Encoder struct {
	path      string
	probePath string
}

// ToolsConfig supplies executable paths from process assembly, never config.json.
// Empty paths use PATH.
type ToolsConfig struct {
	FFmpegPath  string
	FFprobePath string
}

// New builds the encoder over one pair of tool paths.
func New(cfg ToolsConfig) execute.Encoder {
	return Encoder{path: cfg.FFmpegPath, probePath: cfg.FFprobePath}
}

func (f Encoder) paths() (string, string) {
	encoder, probe := f.path, f.probePath
	if encoder == "" {
		encoder = "ffmpeg"
	}
	if probe == "" {
		probe = "ffprobe"
	}
	return encoder, probe
}

// Check verifies both tools before any filesystem changes.
func (f Encoder) Check() error {
	encoder, probe := f.paths()
	for _, tool := range []string{encoder, probe} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("tool unavailable %s: %w", tool, err)
		}
	}
	return nil
}

// Version reports the encoder's own version line, the diagnostic a generation
// credential records beside its facts: it names the binary that wrote a file
// without claiming it as part of the file's identity — a re-encode with a newer
// encoder is the same generation, an encode with other settings is not. An
// unavailable tool reports an empty string rather than failing the generation
// it describes.
func (f Encoder) Version(ctx context.Context) string {
	encoder, _ := f.paths()
	out, err := exec.CommandContext(ctx, encoder, "-version").Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line)
}

// Probe reads the first audio stream of one file.
func (f Encoder) Probe(ctx context.Context, path string) (execute.AudioStream, error) {
	_, probe := f.paths()
	data, err := exec.CommandContext(ctx, probe, "-v", "error", "-select_streams", "a:0", "-show_streams", "-of", "json", path).
		Output()
	if err != nil {
		return execute.AudioStream{}, fmt.Errorf("probe %s: %w", path, err)
	}
	var result struct {
		Streams []execute.AudioStream `json:"streams"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return execute.AudioStream{}, err
	}
	if len(result.Streams) != 1 || result.Streams[0].Channels < 1 {
		return execute.AudioStream{}, fmt.Errorf("no audio stream: %s", path)
	}
	return result.Streams[0], nil
}

// TargetFacts names the encoder binary and the rate-control mode one encoded
// target is written with. It is derived from the same table Encode builds its
// arguments from, so a generation credential can never describe a different
// invocation than the one that ran.
func (f Encoder) TargetFacts(spec reconcile.AudioOutputSpec) (encoder, mode string, err error) {
	target, err := encodedTarget(spec.Codec, spec.Quality)
	if err != nil {
		return "", "", err
	}
	return target.Encoder, target.Mode, nil
}

// Encode writes a staged output and returns its source facts. The caller closes
// the writer and validates the final read-only object before publication.
// The source's tags come with it, whichever container reconciliation picked as
// the source — a declared lossless target, else WAV, else FLAC — and a WAV that
// carries them as legacy Shift-JIS RIFF INFO bytes has those values repaired
// instead of copied as mojibake. Chapters and embedded pictures are not copied:
// each output is one complete track, without container-specific extras.
// AAC uses bitrate-controlled CBR (-b:a), never quality/VBR (-q:a).
// The caller owns a newly created seekable destination handle. Partial output on failure
// belongs to the caller; this adapter never commits, removes or replaces media.
func (f Encoder) Encode(
	ctx context.Context,
	src, dst *os.File,
	spec reconcile.AudioOutputSpec,
) (execute.AudioStream, error) {
	if src == nil || dst == nil || src == dst {
		return execute.AudioStream{}, fmt.Errorf("distinct source and destination handles required")
	}
	stream, err := f.encodeHandles(ctx, src, dst, spec)
	return stream, err
}

func (f Encoder) encodeHandles(
	ctx context.Context,
	src, dst *os.File,
	spec reconcile.AudioOutputSpec,
) (execute.AudioStream, error) {
	input, tagArgs, err := f.sourceProbe(ctx, src)
	if err != nil {
		return execute.AudioStream{}, err
	}
	codecArgs, err := audioEncodingArgs(input, spec)
	if err != nil {
		return execute.AudioStream{}, err
	}
	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel",
		"error",
		"-xerror",
		"-n",
		"-fd",
		"0",
		"-i",
		"fd:",
		"-map",
		"0:a:0",
		"-map_metadata",
		"0",
		"-map_chapters",
		"-1",
		"-vn",
	}
	args = append(args, tagArgs...)
	args = append(args, codecArgs.Args...)
	args = append(args, "-fd", "1", "fd:")
	encoder, _ := f.paths()
	if _, seekErr := src.Seek(0, 0); seekErr != nil {
		return execute.AudioStream{}, seekErr
	}
	if _, seekErr := dst.Seek(0, 0); seekErr != nil {
		return execute.AudioStream{}, seekErr
	}
	cmd := exec.CommandContext(ctx, encoder, args...)
	cmd.Stdin, cmd.Stdout = src, dst
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	if runErr := cmd.Run(); runErr != nil {
		return execute.AudioStream{}, fmt.Errorf("encode: %w: %s", runErr, diagnostic.String())
	}
	return input, nil
}

func (f Encoder) decodeHandle(ctx context.Context, file *os.File) error {
	if _, seekErr := file.Seek(0, 0); seekErr != nil {
		return seekErr
	}
	encoder, _ := f.paths()
	cmd := exec.CommandContext(
		ctx,
		encoder,
		"-nostdin",
		"-v",
		"error",
		"-xerror",
		"-fd",
		"0",
		"-i",
		"fd:",
		"-map",
		"0:a:0",
		"-f",
		"null",
		"-",
	)
	cmd.Stdin = file
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	if err := validationCommand(ctx, cmd, &diagnostic); err != nil {
		return fmt.Errorf("decode output: %w", err)
	}
	return nil
}

func (f Encoder) probeHandle(ctx context.Context, file *os.File) (execute.AudioStream, error) {
	data, err := f.probeData(ctx, file, false, false)
	if err != nil {
		return execute.AudioStream{}, fmt.Errorf("probe handle: %w", err)
	}
	return probeStream(data)
}
