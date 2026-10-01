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
	sec, _, err := noFFprobe(t).AudioFacts(context.Background(), writeFile(t, "a.wav", data))
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
		if _, _, err := noFFprobe(t).AudioFacts(context.Background(), writeFile(t, name, data)); err == nil {
			t.Errorf("%s: expected no header answer", name)
		}
	}
}

// mp3Frame is one MPEG-1 layer III frame at 44.1 kHz, stereo, 1152 samples;
// bitrate index 9 is 128 kbps (417 bytes), 10 is 160 kbps (522 bytes). body is
// placed right after the 32-byte side info.
func mp3Frame(bitrateIndex byte, body []byte) []byte {
	kbps := map[byte]int{9: 128, 10: 160}[bitrateIndex]
	frame := make([]byte, 144*kbps*1000/44100)
	copy(frame, []byte{0xFF, 0xFB, bitrateIndex << 4, 0x00})
	copy(frame[4+32:], body)
	return frame
}

func vbrTag(id string, frames, size uint32) []byte {
	flags := uint32(1)
	if size > 0 {
		flags |= 2
	}
	b := binary.BigEndian.AppendUint32([]byte(id), flags)
	b = binary.BigEndian.AppendUint32(b, frames)
	if size > 0 {
		b = binary.BigEndian.AppendUint32(b, size)
	}
	return b
}

func TestMP3FactsFromHeaders(t *testing.T) {
	id3 := append([]byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 100}, make([]byte, 100)...) // synchsafe 100
	frames := func(n int) []byte {
		var b bytes.Buffer
		for range n {
			b.Write(mp3Frame(9, nil))
		}
		return b.Bytes()
	}
	cases := map[string]struct {
		file    []byte
		sec     float64
		bitrate int64
	}{
		// Xing: frame count for the length, ffmpeg's rounded average for the rate.
		"xing vbr after an ID3v2 tag": {
			file:    bytes.Join([][]byte{id3, mp3Frame(9, vbrTag("Xing", 100, 41700)), frames(1)}, nil),
			sec:     100 * 1152 / 44100.0,
			bitrate: 127706, // round(41700 * 8 * 44100 / (100 * 1152))
		},
		// LAME's Info: constant bitrate, so the frame header's rate.
		"info cbr": {
			file:    bytes.Join([][]byte{mp3Frame(9, vbrTag("Info", 50, 20850)), frames(1)}, nil),
			sec:     50 * 1152 / 44100.0,
			bitrate: 128000,
		},
		// No header: the audio bytes over the frame header's rate.
		"plain cbr": {
			file:    frames(50),
			sec:     50 * 417 * 8 / 128000.0,
			bitrate: 128000,
		},
	}
	for name, c := range cases {
		sec, bitrate, err := noFFprobe(t).AudioFacts(context.Background(), writeFile(t, "a.mp3", c.file))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		assertSeconds(t, sec, c.sec)
		if bitrate != c.bitrate {
			t.Errorf("%s: bitrate = %d, want %d", name, bitrate, c.bitrate)
		}
	}
}

func TestMP3RateChangeWithoutHeaderIsNotGuessed(t *testing.T) {
	// VBR without a Xing header: ffprobe takes its rate from a decoded frame,
	// so the header does not answer.
	file := bytes.Join([][]byte{mp3Frame(9, nil), mp3Frame(9, nil), mp3Frame(10, nil), mp3Frame(9, nil)}, nil)
	if _, _, err := noFFprobe(t).AudioFacts(context.Background(), writeFile(t, "vbr.mp3", file)); err == nil {
		t.Fatal("expected no header answer for a rate change without a VBR header")
	}
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
	sec, _, err := media.New("").AudioFacts(context.Background(), path)
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

// timesBox is a movie (mvhd) or media (mdhd) header, which share their layout
// up to the duration: version 0 has 32-bit times, version 1 64-bit ones.
func timesBox(typ string, version byte, timescale uint32, duration uint64) []byte {
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
	return box(typ, append(b, make([]byte, 80)...))
}

// trak is one track: its media header, then sample tables the parser must
// never need.
func trak(mdhd []byte) []byte {
	return box("trak", box("mdia", mdhd, box("minf", make([]byte, 4096))))
}

func TestMP4FactsFromHeaders(t *testing.T) {
	ftyp := box("ftyp", []byte("M4A \x00\x00\x00\x00M4A mp42isom"))
	// A 64-bit mdat size: size 1, then the large size after the type.
	large := binary.BigEndian.AppendUint32(nil, 1)
	large = append(large, "mdat"...)
	large = binary.BigEndian.AppendUint64(large, 16+2000)
	large = append(large, make([]byte, 2000)...)
	mdat := box("mdat", make([]byte, 5000))

	cases := map[string]struct {
		file    []byte
		sec     float64
		bitrate int64
	}{
		// ffmpeg and qaac without faststart: moov after the audio. The movie
		// header is the trimmed length; the track's, with priming, is longer.
		"moov at the end": {
			file: bytes.Join([][]byte{ftyp, mdat, box("moov",
				timesBox("mvhd", 0, 1000, 6979), trak(timesBox("mdhd", 0, 44100, 44100*7)))}, nil),
			sec:     6.979,
			bitrate: 5714, // 5000 * 8 * 44100 / (44100 * 7), truncated
		},
		"moov first, 64-bit headers": {
			file: bytes.Join([][]byte{ftyp, box("moov",
				timesBox("mvhd", 1, 44100, 44100*20), trak(timesBox("mdhd", 1, 48000, 48000*20))), mdat}, nil),
			sec:     20,
			bitrate: 2000,
		},
		"64-bit mdat size": {
			file: bytes.Join([][]byte{ftyp, large, box("moov",
				timesBox("mvhd", 0, 1000, 8000), trak(timesBox("mdhd", 0, 1000, 8000)))}, nil),
			sec:     8,
			bitrate: 2000,
		},
	}
	for name, c := range cases {
		sec, bitrate, err := noFFprobe(t).AudioFacts(context.Background(), writeFile(t, "a.m4a", c.file))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		assertSeconds(t, sec, c.sec)
		if bitrate != c.bitrate {
			t.Errorf("%s: bitrate = %d, want %d", name, bitrate, c.bitrate)
		}
	}
}

func TestMP4WithoutExactFactsIsNotGuessed(t *testing.T) {
	track := trak(timesBox("mdhd", 0, 1000, 1000))
	cases := map[string][]byte{
		// A fragmented file's movie header carries no duration.
		"fragmented": bytes.Join([][]byte{
			box("moov", timesBox("mvhd", 0, 1000, 0), track), box("moof"), box("mdat", make([]byte, 100)),
		}, nil),
		// Two tracks share the mdat: its payload is not one track's samples.
		"two tracks": bytes.Join([][]byte{
			box("moov", timesBox("mvhd", 0, 1000, 1000), track, track), box("mdat", make([]byte, 100)),
		}, nil),
	}
	for name, file := range cases {
		if _, _, err := noFFprobe(t).AudioFacts(context.Background(), writeFile(t, "a.m4a", file)); err == nil {
			t.Errorf("%s: expected no header answer", name)
		}
	}
}
