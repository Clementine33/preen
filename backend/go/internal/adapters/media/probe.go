// Package media measures the media facts the inventory records beside an
// entry: image dimensions, audio duration and bitrate - from the file's header
// where it can, through ffprobe for the rest. It implements the inventory
// prober port and the planner's bitrate port (ADR 0009) and never writes a
// file.
package media

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	_ "golang.org/x/image/webp" // header registration for image.DecodeConfig
)

// Prober is one ffprobe configuration. An empty path uses the executable on
// PATH, the same default the encoder adapter takes.
type Prober struct {
	ffprobePath string
}

// New builds the prober over one ffprobe path.
func New(ffprobePath string) *Prober {
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	return &Prober{ffprobePath: ffprobePath}
}

// ImageDimensions reads only the image header (image.DecodeConfig), never the
// pixels — the boundary ADR 0015 §2 draws for media facts.
func (p *Prober) ImageDimensions(pathPosix string) (int, int, error) {
	f, err := os.Open(filepath.FromSlash(pathPosix))
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, fmt.Errorf("implausible dimensions %dx%d: %s", cfg.Width, cfg.Height, pathPosix)
	}
	return cfg.Width, cfg.Height, nil
}

// AudioFacts measures a file's duration and bitrate (bits per second). The
// header answers both for WAV, MP3 and m4a (audio_header.go); a file it cannot
// answer in full - another format, an unrecognized header, an encoded file
// whose bitrate the header cannot give exactly - takes one ffprobe call for
// both. A zero is unknown; an error means neither could be measured.
func (p *Prober) AudioFacts(ctx context.Context, pathPosix string) (float64, int64, error) {
	if sec, bitrate, err := nativeAudio(pathPosix); err == nil && (bitrate > 0 || !carriesBitrate(pathPosix)) {
		return sec, bitrate, nil
	}
	return p.ffprobeAudio(ctx, pathPosix)
}

// ProbeBitrate measures one file's bitrate the same way, for the planner's
// gate, with the ffprobe the planning configuration names (empty: PATH).
func ProbeBitrate(ctx context.Context, ffprobePath, pathPosix string) (int64, error) {
	_, bitrate, err := New(ffprobePath).AudioFacts(ctx, pathPosix)
	if err == nil && bitrate <= 0 {
		err = fmt.Errorf("no bitrate reported: %s", pathPosix)
	}
	return bitrate, err
}

// carriesBitrate names the encoded containers whose bitrate is a fact the
// planner compares against a target; a lossless file's rate decides nothing.
func carriesBitrate(pathPosix string) bool {
	switch strings.ToLower(filepath.Ext(pathPosix)) {
	case ".mp3", ".aac", ".m4a", ".opus":
		return true
	}
	return false
}

// ffprobeAudio asks ffprobe for both facts in one call.
//
//   - Duration: the stream's own is exact; a stream that declares none falls
//     back to the container's.
//   - Bitrate: the stream's own (MP3, AAC); Ogg/Opus declares none, so the
//     container's average - for the CBR the executor writes it sits a little
//     above the target, which the satisfaction comparison can use.
//
//nolint:gosec // Tool path comes from local config; arguments are passed directly without a shell.
func (p *Prober) ffprobeAudio(ctx context.Context, pathPosix string) (float64, int64, error) {
	data, err := exec.CommandContext(ctx, p.ffprobePath, "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=duration,bit_rate:format=duration,bit_rate", "-of", "json",
		"-i", filepath.FromSlash(pathPosix)).Output()
	if err != nil {
		return 0, 0, err
	}
	type facts struct {
		Duration string `json:"duration"`
		Bitrate  string `json:"bit_rate"`
	}
	var result struct {
		Streams []facts `json:"streams"`
		Format  facts   `json:"format"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, 0, err
	}
	if len(result.Streams) != 1 {
		return 0, 0, fmt.Errorf("no audio stream: %s", pathPosix)
	}
	stream := result.Streams[0]
	var sec float64
	for _, candidate := range []string{stream.Duration, result.Format.Duration} {
		if v, err := strconv.ParseFloat(candidate, 64); err == nil && v > 0 {
			sec = v
			break
		}
	}
	var bitrate int64
	for _, candidate := range []string{stream.Bitrate, result.Format.Bitrate} {
		if candidate == "" {
			continue
		}
		if v, err := strconv.ParseInt(candidate, 10, 64); err == nil && v > 0 {
			bitrate = v
		}
		break // a declared stream rate is the fact, even when unparsable
	}
	if sec <= 0 && bitrate <= 0 {
		return 0, 0, fmt.Errorf("no duration or bitrate reported: %s", pathPosix)
	}
	return sec, bitrate, nil
}
