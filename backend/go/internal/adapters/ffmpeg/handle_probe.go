package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/onsei/organizer/backend/internal/conversion/execute"
)

// sourceProbe combines WAV stream facts with raw RIFF INFO tags in one probe.
// Legacy Shift-JIS values must reach parseTagPairs before JSON UTF-8 repair;
// repairedTagArgs preserves the existing metadata candidate selection. Only tag
// parsing is best-effort; unusable tags cannot discard valid stream facts.
func (f Encoder) sourceProbe(ctx context.Context, file *os.File) (execute.AudioStream, []string, error) {
	var header [12]byte
	_, err := file.ReadAt(header[:], 0)
	wav := err == nil && string(header[:4]) == "RIFF" && string(header[8:]) == "WAVE"
	data, err := f.probeData(ctx, file, wav, false)
	if err != nil {
		return execute.AudioStream{}, nil, err
	}
	stream, err := probeStream(data)
	if err != nil {
		return execute.AudioStream{}, nil, err
	}
	var tags []string
	if wav {
		if pairs, tagErr := parseTagPairs(data); tagErr == nil {
			tags = repairedTagArgs(pairs)
		}
	}
	return stream, tags, nil
}

func probeStream(data []byte) (execute.AudioStream, error) {
	var result struct {
		Streams []execute.AudioStream `json:"streams"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return execute.AudioStream{}, err
	}
	if len(result.Streams) != 1 || result.Streams[0].Channels < 1 {
		return execute.AudioStream{}, fmt.Errorf("no audio stream")
	}
	return result.Streams[0], nil
}

func (f Encoder) probeData(ctx context.Context, file *os.File, tags, strict bool) ([]byte, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}
	_, probe := f.paths()
	args := []string{"-v", "error", "-select_streams", "a:0", "-show_streams", "-of", "json"}
	if tags {
		args = []string{
			"-v",
			"error",
			"-select_streams",
			"a:0",
			"-show_entries",
			"stream:format_tags",
			"-of",
			"json=string_validation=ignore",
		}
	}
	args = append(args, "-fd", "0", "fd:")
	cmd := exec.CommandContext(ctx, probe, args...)
	cmd.Stdin = file
	var output, diagnostic bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &diagnostic
	if strict {
		if err := validationCommand(ctx, cmd, &diagnostic); err != nil {
			return nil, fmt.Errorf("probe output: %w", err)
		}
	} else if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("probe source: %w: %s", err, diagnostic.String())
	}
	return output.Bytes(), nil
}

// Validation commands use -v error: even an exit-zero diagnostic means the
// decoder or demuxer found damage. Cancellation takes precedence over stderr.
func validationCommand(ctx context.Context, cmd *exec.Cmd, diagnostic *bytes.Buffer) error {
	err := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("%w: %s", err, diagnostic.String())
	}
	if message := strings.TrimSpace(diagnostic.String()); message != "" {
		return fmt.Errorf("validation diagnostic: %s", message)
	}
	return nil
}
