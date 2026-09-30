// Package media measures the scan-time media facts the inventory records
// beside an entry: image dimensions from the file's header, audio duration
// through ffprobe. It implements the inventory prober port (ADR 0009) and
// never writes a file.
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
// pixels — the same boundary ADR 0015 §2 draws for the scan.
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

// AudioDurationSec probes a file's duration. The stream's own duration is the
// exact fact; a stream that declares none (mp3) falls back to the container's
// average, the same fallback order the bitrate probe uses for its rate.
//
//nolint:gosec // Tool path comes from local config; arguments are passed directly without a shell.
func (p *Prober) AudioDurationSec(ctx context.Context, pathPosix string) (float64, error) {
	data, err := exec.CommandContext(ctx, p.ffprobePath, "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=duration:format=duration", "-of", "json",
		"-i", filepath.FromSlash(pathPosix)).Output()
	if err != nil {
		return 0, err
	}
	var result struct {
		Streams []struct {
			Duration string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, err
	}
	if len(result.Streams) == 0 {
		return 0, fmt.Errorf("no audio stream: %s", pathPosix)
	}
	for _, candidate := range []string{result.Streams[0].Duration, result.Format.Duration} {
		if candidate == "" || candidate == "N/A" {
			continue
		}
		sec, err := strconv.ParseFloat(candidate, 64)
		if err == nil && sec > 0 {
			return sec, nil
		}
	}
	return 0, fmt.Errorf("no duration reported: %s", pathPosix)
}
