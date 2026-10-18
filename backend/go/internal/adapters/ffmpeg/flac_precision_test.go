package ffmpeg_test

import (
	"bytes"
	"encoding/binary"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/ffmpeg"
	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
)

func TestFFmpegFLACSourcePrecision(t *testing.T) {
	for _, tc := range []struct {
		codec string
		bits  string
	}{
		{"pcm_f32le", "24"},
		{"pcm_f64le", "24"},
		{"pcm_s16le", "16"},
		{"pcm_s24le", "24"},
		{"pcm_s32le", ""},
	} {
		t.Run(tc.codec, func(t *testing.T) {
			src := filepath.Join(t.TempDir(), "source.wav")
			runFFmpeg(t, "-f", "lavfi", "-i", "aevalsrc=1.25|-0.5:s=96000:d=0.1", "-c:a", tc.codec, src)
			encoder := ffmpeg.New(ffmpeg.ToolsConfig{})
			dst := filepath.Join(t.TempDir(), "output.stage")
			spec := reconcile.AudioOutputSpec{Codec: reconcile.CodecFlac}
			err := encodePaths(encoder, t.Context(), src, dst, spec)
			if tc.bits == "" {
				if err == nil || !strings.Contains(err.Error(), "FLAC target cannot preserve source precision") {
					t.Fatalf("32-bit integer source: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			stream := probeStream(t, dst)
			if stream.Codec != "flac" || stream.Bits != tc.bits {
				t.Fatalf("output = %+v, want %s-bit FLAC", stream, tc.bits)
			}
			if !strings.HasPrefix(tc.codec, "pcm_f") {
				return
			}
			// Full-scale input clips, while -0.5 stays unchanged (no normalization).
			pcm, err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-i", dst,
				"-f", "s32le", "-c:a", "pcm_s32le", "-").Output()
			if err != nil {
				t.Fatal(err)
			}
			var samples [2]int32
			if err := binary.Read(bytes.NewReader(pcm), binary.LittleEndian, &samples); err != nil {
				t.Fatal(err)
			}
			left, right := samples[0]>>8, samples[1]>>8
			if left != 8388607 || right != -4194304 {
				t.Fatalf("decoded samples = (%d, %d), want (8388607, -4194304)", left, right)
			}
		})
	}
}
