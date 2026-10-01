package media_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/media"
)

// noFFprobe points the fallback at nothing: a duration answered here was read
// from the header.
func noFFprobe(t *testing.T) *media.Prober {
	t.Helper()
	return media.New(filepath.Join(t.TempDir(), "no-ffprobe"))
}

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(path)
}

func chunk(id string, body []byte) []byte {
	//nolint:gosec // test chunks are a few hundred KiB at most
	out := append([]byte(id), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...)
	out = append(out, body...)
	if len(body)%2 == 1 {
		out = append(out, 0) // word-alignment pad
	}
	return out
}

func riff(chunks ...[]byte) []byte {
	body := []byte("WAVE")
	for _, c := range chunks {
		body = append(body, c...)
	}
	return chunk("RIFF", body)
}

// pcmFmt is 44.1 kHz stereo 16-bit: 176400 bytes a second.
func pcmFmt() []byte {
	b := binary.LittleEndian.AppendUint16(nil, 1)  // PCM
	b = binary.LittleEndian.AppendUint16(b, 2)     // channels
	b = binary.LittleEndian.AppendUint32(b, 44100) // sample rate
	b = binary.LittleEndian.AppendUint32(b, 176400)
	b = binary.LittleEndian.AppendUint16(b, 4)
	return binary.LittleEndian.AppendUint16(b, 16)
}

func assertSeconds(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("duration = %v, want %v", got, want)
	}
}

func TestWAVDurationFromHeader(t *testing.T) {
	// An odd-sized chunk before fmt exercises the pad byte; the data size over
	// the byte rate is the length.
	data := riff(chunk("LIST", []byte("abc")), chunk("fmt ", pcmFmt()), chunk("data", make([]byte, 352800)))
	sec, err := noFFprobe(t).AudioDurationSec(context.Background(), writeFile(t, "a.wav", data))
	if err != nil {
		t.Fatal(err)
	}
	assertSeconds(t, sec, 2)
}

func TestWAVWithoutUsableSizeIsNotGuessed(t *testing.T) {
	// RF64 carries its size elsewhere; a streamed data chunk claims all ones.
	// Neither is answered from the header, and with no ffprobe it fails.
	rf64 := riff(chunk("fmt ", pcmFmt()), chunk("data", nil))
	copy(rf64, "RF64")
	streamed := riff(chunk("fmt ", pcmFmt()))
	streamed = append(streamed, []byte("data\xff\xff\xff\xff")...)
	for name, data := range map[string][]byte{"rf64.wav": rf64, "streamed.wav": streamed} {
		if _, err := noFFprobe(t).AudioDurationSec(context.Background(), writeFile(t, name, data)); err == nil {
			t.Errorf("%s: expected no header answer", name)
		}
	}
}

// mp3Frame is one MPEG-1 layer III frame, 128 kbps, 44.1 kHz, stereo: 417
// bytes, 1152 samples. body is placed right after the 32-byte side info.
func mp3Frame(body []byte) []byte {
	frame := make([]byte, 417)
	copy(frame, []byte{0xFF, 0xFB, 0x90, 0x00})
	copy(frame[4+32:], body)
	return frame
}

func TestMP3DurationFromXingHeader(t *testing.T) {
	xing := append([]byte("Xing"), 0, 0, 0, 1) // flags: frame count present
	xing = binary.BigEndian.AppendUint32(xing, 100)
	var file bytes.Buffer
	// An ID3v2 tag of 100 bytes (synchsafe size) precedes the audio.
	file.Write([]byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 100})
	file.Write(make([]byte, 100))
	file.Write(mp3Frame(xing))
	file.Write(mp3Frame(nil))

	sec, err := noFFprobe(t).AudioDurationSec(context.Background(), writeFile(t, "vbr.mp3", file.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	assertSeconds(t, sec, 100*1152/44100.0)
}

func TestMP3ConstantBitrateEstimate(t *testing.T) {
	// No VBR header: the audio bytes over the bitrate.
	var file bytes.Buffer
	for range 50 {
		file.Write(mp3Frame(nil))
	}
	sec, err := noFFprobe(t).AudioDurationSec(context.Background(), writeFile(t, "cbr.mp3", file.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	assertSeconds(t, sec, 50*417*8/128000.0)
}

func TestAudioDurationFallsBackToFFprobe(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "track.flac"))
	output, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i",
		"sine=frequency=440:duration=2", path).CombinedOutput()
	if err != nil {
		t.Fatalf("generate flac: %v: %s", err, output)
	}
	sec, err := media.New("").AudioDurationSec(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if sec < 1.9 || sec > 2.1 {
		t.Errorf("expected ~2s, got %v", sec)
	}
}

func box(typ string, body ...[]byte) []byte {
	var payload []byte
	for _, b := range body {
		payload = append(payload, b...)
	}
	//nolint:gosec // test boxes are a few KiB at most
	out := binary.BigEndian.AppendUint32(nil, uint32(8+len(payload)))
	return append(append(out, typ...), payload...)
}

// mvhd is a movie header: version 0 has 32-bit times, version 1 64-bit ones.
func mvhd(version byte, timescale uint32, duration uint64) []byte {
	b := []byte{version, 0, 0, 0}
	if version == 1 {
		b = append(b, make([]byte, 16)...) // creation, modification
		b = binary.BigEndian.AppendUint32(b, timescale)
		b = binary.BigEndian.AppendUint64(b, duration)
	} else {
		b = append(b, make([]byte, 8)...)
		b = binary.BigEndian.AppendUint32(b, timescale)
		b = binary.BigEndian.AppendUint32(b, uint32(duration)) //nolint:gosec // test values fit
	}
	return box("mvhd", append(b, make([]byte, 80)...))
}

// moovWith stands for a real moov: the movie header, then a track whose
// sample tables the parser must never need.
func moovWith(header []byte) []byte {
	return box("moov", header, box("trak", box("mdia", make([]byte, 4096))))
}

func TestMP4DurationFromMovieHeader(t *testing.T) {
	ftyp := box("ftyp", []byte("M4A \x00\x00\x00\x00M4A mp42isom"))
	// A 64-bit mdat size: size 1, then the large size after the type.
	large := binary.BigEndian.AppendUint32(nil, 1)
	large = append(large, "mdat"...)
	large = binary.BigEndian.AppendUint64(large, 16+2000)
	large = append(large, make([]byte, 2000)...)

	cases := map[string]struct {
		file []byte
		want float64
	}{
		// ffmpeg and qaac without faststart: moov after the audio.
		"moov at the end": {
			file: bytes.Join([][]byte{ftyp, box("mdat", make([]byte, 5000)), moovWith(mvhd(0, 1000, 253938))}, nil),
			want: 253.938,
		},
		"moov first, 64-bit header": {
			file: bytes.Join([][]byte{ftyp, moovWith(mvhd(1, 44100, 44100*90)), box("mdat", make([]byte, 5000))}, nil),
			want: 90,
		},
		"64-bit mdat size": {
			file: bytes.Join([][]byte{ftyp, large, moovWith(mvhd(0, 48000, 48000*12))}, nil),
			want: 12,
		},
	}
	for name, c := range cases {
		sec, err := noFFprobe(t).AudioDurationSec(context.Background(), writeFile(t, "a.m4a", c.file))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		assertSeconds(t, sec, c.want)
	}
}

func TestMP4WithoutMovieDurationIsNotGuessed(t *testing.T) {
	// A fragmented file's movie header carries no duration: ffprobe's job.
	file := bytes.Join([][]byte{moovWith(mvhd(0, 1000, 0)), box("moof"), box("mdat", make([]byte, 100))}, nil)
	if _, err := noFFprobe(t).AudioDurationSec(context.Background(), writeFile(t, "frag.m4a", file)); err == nil {
		t.Fatal("expected no header answer for a fragmented file")
	}
}
