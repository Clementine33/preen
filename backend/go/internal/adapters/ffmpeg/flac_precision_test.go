package ffmpeg_test

import (
	"bytes"
	"encoding/binary"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/ffmpeg"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
)

// TestFFmpegFLACSourcePrecision pins the FLAC target's sample representation:
// an integer source keeps its own precision up to 32 bits, a floating-point
// source becomes 32-bit integers, and nothing dithers, normalizes or pre-scans
// peaks. The encoder writes more than 24 bits per sample only under -strict
// experimental, so these cases also guard that flag.
func TestFFmpegFLACSourcePrecision(t *testing.T) {
	for _, tc := range []struct {
		codec string
		bits  string
	}{
		{"pcm_s16le", "16"},
		{"pcm_s24le", "24"},
		{"pcm_s32le", "32"},
		{"pcm_f32le", "32"},
		{"pcm_f64le", "32"},
	} {
		t.Run(tc.codec, func(t *testing.T) {
			src := filepath.Join(t.TempDir(), "source.wav")
			runFFmpeg(t, "-f", "lavfi", "-i", "aevalsrc=1.25|-0.5:s=96000:d=0.1", "-c:a", tc.codec, src)
			encoder := ffmpeg.New(ffmpeg.ToolsConfig{})
			dst := filepath.Join(t.TempDir(), "output.stage")
			spec := reconcile.AudioOutputSpec{Codec: reconcile.CodecFlac}
			if err := encodePaths(encoder, t.Context(), src, dst, spec); err != nil {
				t.Fatal(err)
			}
			stream := probeStream(t, dst)
			if stream.Codec != "flac" || stream.Bits != tc.bits {
				t.Fatalf("output = %+v, want %s-bit FLAC", stream, tc.bits)
			}
			got := decodeS32(t, dst)
			if !strings.HasPrefix(tc.codec, "pcm_f") {
				// An integer source survives the round trip sample for sample.
				if want := decodeS32(t, src); !bytes.Equal(got, want) {
					t.Fatalf("integer round trip changed samples")
				}
				return
			}
			// Full-scale input clips, while -0.5 stays exact (no normalization).
			var samples [2]int32
			if err := binary.Read(bytes.NewReader(got), binary.LittleEndian, &samples); err != nil {
				t.Fatal(err)
			}
			if samples[0] != 2147483647 || samples[1] != -1073741824 {
				t.Fatalf("decoded samples = (%d, %d), want (2147483647, -1073741824)", samples[0], samples[1])
			}
		})
	}
}

// decodeS32 reads one file back as raw 32-bit samples of the first stream.
func decodeS32(t *testing.T, path string) []byte {
	t.Helper()
	pcm, err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-i", path,
		"-f", "s32le", "-c:a", "pcm_s32le", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	return pcm
}
