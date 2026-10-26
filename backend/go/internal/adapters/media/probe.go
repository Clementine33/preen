// Package media implements media probing and image scaling for inventory,
// conversion planning and thumbnail consumers. It reads image dimensions,
// audio duration and bitrate from native headers where possible, using ffprobe
// otherwise; its thumbnail engine decodes and scales images into encoded bytes.
//
// Consumers own enrichment scheduling, conversion policy and thumbnail caching.
// This adapter reads source files and returns facts or bytes without publishing
// files or owning their cache lifecycle.
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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "golang.org/x/image/webp" // header registration for image.DecodeConfig

	"github.com/Clementine33/preen/backend/internal/adapters/toolprocess"
)

// probeTimeout bounds one file's probe: a header read is a few KiB and ffprobe
// a little more, so only a stalled share or a hung tool runs into it.
const probeTimeout = 20 * time.Second

// Prober is one ffprobe configuration. An empty path uses the executable on
// PATH, the same default the encoder adapter takes.
type Prober struct {
	ffprobePath string
	// Timeout bounds each file's probe, header read and ffprobe together; zero
	// means probeTimeout. A probe past it fails, so the fact stays unknown.
	Timeout time.Duration
}

// New builds the prober over one ffprobe path.
func New(ffprobePath string) *Prober {
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	return &Prober{ffprobePath: ffprobePath}
}

// deadline gives one file's probe its timeout.
func (p *Prober) deadline(ctx context.Context, pathPosix string) (context.Context, context.CancelFunc) {
	d := p.Timeout
	if d <= 0 {
		d = probeTimeout
	}
	return context.WithTimeoutCause(ctx, d, fmt.Errorf("probe timed out after %s: %s", d, pathPosix))
}

// bounded returns a blocking file read's result, or gives up when ctx ends. A
// read stuck on a stalled share cannot be interrupted, so an abandoned one
// finishes in the background and its result is dropped.
// ponytail: a share that never answers keeps one goroutine and descriptor per
// abandoned file until it does; the pass retries those files only once per kick.
func bounded[T any](ctx context.Context, read func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1)
	go func() {
		v, err := read()
		done <- result{v, err}
	}()
	select {
	case r := <-done:
		return r.v, r.err
	case <-ctx.Done():
		var zero T
		return zero, context.Cause(ctx)
	}
}

// ImageDimensions reads only the image header (image.DecodeConfig), never the
// pixels — the boundary ADR 0013 draws for media facts.
func (p *Prober) ImageDimensions(ctx context.Context, pathPosix string) (int, int, error) {
	ctx, cancel := p.deadline(ctx, pathPosix)
	defer cancel()
	size, err := bounded(ctx, func() ([2]int, error) {
		w, h, err := imageDimensions(pathPosix)
		return [2]int{w, h}, err
	})
	return size[0], size[1], err
}

func imageDimensions(pathPosix string) (int, int, error) {
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
// both. A zero is unknown; an error means neither could be measured. One
// timeout covers both steps.
func (p *Prober) AudioFacts(ctx context.Context, pathPosix string) (float64, int64, error) {
	ctx, cancel := p.deadline(ctx, pathPosix)
	defer cancel()
	type facts struct {
		sec     float64
		bitrate int64
	}
	native, err := bounded(ctx, func() (facts, error) {
		sec, bitrate, err := nativeAudio(pathPosix)
		return facts{sec, bitrate}, err
	})
	if err == nil && (native.bitrate > 0 || !carriesBitrate(pathPosix)) {
		return native.sec, native.bitrate, nil
	}
	if ctx.Err() != nil {
		return 0, 0, context.Cause(ctx)
	}
	return p.ffprobeAudio(ctx, pathPosix)
}

// ProbeBitrate measures one file's bitrate using the default PATH tool.
func ProbeBitrate(ctx context.Context, pathPosix string) (int64, error) {
	return New("").ProbeBitrate(ctx, pathPosix)
}

// ProbeBitrate measures bitrate with this prober's native readers and fallback.
func (p *Prober) ProbeBitrate(ctx context.Context, pathPosix string) (int64, error) {
	_, bitrate, err := p.AudioFacts(ctx, pathPosix)
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
func (p *Prober) ffprobeAudio(ctx context.Context, pathPosix string) (float64, int64, error) {
	cmd := toolprocess.CommandContext(ctx, p.ffprobePath, "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=duration,bit_rate:format=duration,bit_rate", "-of", "json",
		"-i", filepath.FromSlash(pathPosix))
	// A killed ffprobe whose output pipe something else still holds must not
	// keep Output waiting.
	cmd.WaitDelay = time.Second
	data, err := cmd.Output()
	if ctx.Err() != nil {
		return 0, 0, context.Cause(ctx)
	}
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
