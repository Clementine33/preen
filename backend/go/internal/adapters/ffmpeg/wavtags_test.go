package ffmpeg_test

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"

	"github.com/onsei/organizer/backend/internal/adapters/ffmpeg"
	"github.com/onsei/organizer/backend/internal/conversion/execute"
	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
)

// riffInfo is one sub-chunk of a legacy RIFF INFO tag list, its value the raw
// bytes the file declares — which is how these files carry Shift-JIS text.
type riffInfo struct {
	id    string
	value []byte
}

// id3Frame is one ID3v2.3 text frame, written UTF-8 encoded.
type id3Frame struct {
	id   string
	text string
}

func shiftJIS(t *testing.T, text string) []byte {
	t.Helper()
	encoded, _, err := transform.Bytes(japanese.ShiftJIS.NewEncoder(), []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// writeTaggedWAV writes a canonical PCM WAV whose tag chunks a test controls
// completely. Neither candidate can be produced by ffmpeg: its WAV muxer writes
// only a LIST/INFO chunk, always UTF-8, and never a trailing id3 chunk — so the
// container shapes and encodings that decide this behaviour are assembled by
// hand here.
//
// id3First places the id3 chunk before or after LIST/INFO, which is what decides
// which value ffmpeg's first-match metadata lookup ends up copying.
func writeTaggedWAV(t *testing.T, path string, info []riffInfo, frames []id3Frame, id3First bool) {
	t.Helper()
	const (
		sampleRate = 48000
		channels   = 2
		bits       = 16
		seconds    = 1
	)
	blockAlign := channels * bits / 8
	samples := sampleRate * seconds
	pcm := make([]byte, samples*blockAlign)
	for i := range samples {
		sample := int16(3000 * math.Sin(2*math.Pi*440*float64(i)/sampleRate))
		for c := range channels {
			//nolint:gosec // Two's complement is exactly what PCM samples are.
			binary.LittleEndian.PutUint16(pcm[i*blockAlign+c*2:], uint16(sample))
		}
	}
	format := make([]byte, 16)
	binary.LittleEndian.PutUint16(format[0:], 1)
	binary.LittleEndian.PutUint16(format[2:], channels)
	binary.LittleEndian.PutUint32(format[4:], sampleRate)
	binary.LittleEndian.PutUint32(format[8:], uint32(sampleRate*blockAlign))
	binary.LittleEndian.PutUint16(format[12:], uint16(blockAlign))
	binary.LittleEndian.PutUint16(format[14:], uint16(bits))

	var list, id3 []byte
	if len(info) > 0 {
		body := []byte("INFO")
		for _, item := range info {
			body = append(body, riffChunk(item.id, item.value)...)
		}
		list = riffChunk("LIST", body)
	}
	if len(frames) > 0 {
		id3 = riffChunk("id3 ", id3Tag(frames))
	}

	body := []byte("WAVE")
	body = append(body, riffChunk("fmt ", format)...)
	body = append(body, riffChunk("data", pcm)...)
	if id3First {
		body = append(body, id3...)
		body = append(body, list...)
	} else {
		body = append(body, list...)
		body = append(body, id3...)
	}
	out := append([]byte("RIFF"), uint32le(len(body))...)
	out = append(out, body...)
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

func riffChunk(id string, body []byte) []byte {
	out := make([]byte, 0, 8+len(body)+1)
	out = append(out, id...)
	out = append(out, uint32le(len(body))...)
	out = append(out, body...)
	if len(body)%2 == 1 {
		out = append(out, 0)
	}
	return out
}

// id3Tag builds an unsynchronised-free ID3v2.3 tag whose text frames are UTF-8.
func id3Tag(frames []id3Frame) []byte {
	var body []byte
	for _, frame := range frames {
		text := append([]byte{3}, frame.text...) // 3 = UTF-8
		header := make([]byte, 10)
		copy(header, frame.id)
		copy(header[4:], uint32be(len(text)))
		body = append(body, header...)
		body = append(body, text...)
	}
	tag := []byte("ID3\x03\x00\x00")
	tag = append(tag, []byte{
		byte(len(body) >> 21 & 0x7f),
		byte(len(body) >> 14 & 0x7f),
		byte(len(body) >> 7 & 0x7f),
		byte(len(body) & 0x7f),
	}...)
	return append(tag, body...)
}

// uint32le writes a fixture chunk size as a little-endian word.
func uint32le(size int) []byte {
	out := make([]byte, 4)
	//nolint:gosec // Fixture chunks are a few hundred bytes, far below 4 GiB.
	binary.LittleEndian.PutUint32(out, uint32(size))
	return out
}

// uint32be writes a fixture frame size as a big-endian word.
func uint32be(size int) []byte {
	out := make([]byte, 4)
	//nolint:gosec // Fixture frames are a few dozen bytes, far below 4 GiB.
	binary.BigEndian.PutUint32(out, uint32(size))
	return out
}

func tagEncoder(t *testing.T) execute.Encoder {
	t.Helper()
	encoder := ffmpeg.New(execute.ToolsConfig{})
	if err := encoder.Check(); err != nil {
		t.Fatal(err)
	}
	return encoder
}

func encodeAllTargets(
	t *testing.T,
	encoder execute.Encoder,
	src string,
	check func(t *testing.T, codec reconcile.Codec, tags map[string]string),
) {
	t.Helper()
	for _, codec := range []reconcile.Codec{
		reconcile.CodecWav, reconcile.CodecFlac, reconcile.CodecMp3, reconcile.CodecAac, reconcile.CodecOpus,
	} {
		t.Run(string(codec), func(t *testing.T) {
			spec := reconcile.AudioOutputSpec{Codec: codec}
			if !spec.Lossless() {
				spec.Quality = &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 192}
			}
			dst := filepath.Join(t.TempDir(), "output"+reconcile.ExtForCodec(codec))
			if err := encoder.Encode(t.Context(), src, dst, spec); err != nil {
				t.Fatal(err)
			}
			check(t, codec, probeTags(t, dst))
		})
	}
}

// TestFFmpegRepairsLegacyTagBytes pins what a conversion does with a WAV whose
// tags are legacy Shift-JIS RIFF INFO bytes: each value reaches the output as
// the text it denotes, in every target container, instead of as the mojibake
// ffmpeg copies verbatim.
//
// "表の題名" is the load-bearing case: 0x5c is both the second byte of 表 and the
// byte ffprobe escapes, so a value survives only if the escape is undone
// byte-for-byte before the decoder runs. The comment value carries the same trap
// alongside a newline. Values that are already ASCII are left alone.
func TestFFmpegRepairsLegacyTagBytes(t *testing.T) {
	encoder := tagEncoder(t)
	src := filepath.Join(t.TempDir(), "legacy.wav")
	writeTaggedWAV(t, src, []riffInfo{
		{id: "INAM", value: shiftJIS(t, "表の題名")},
		{id: "IART", value: shiftJIS(t, "伊ヶ崎綾香")},
		{id: "IGNR", value: shiftJIS(t, "音声作品")},
		{id: "ICRD", value: []byte("2023")},
		{id: "ITRK", value: []byte("1")},
		{id: "ICMT", value: append(shiftJIS(t, "あ\\"), '\n')},
	}, nil, false)

	encodeAllTargets(t, encoder, src, func(t *testing.T, codec reconcile.Codec, tags map[string]string) {
		if tags["title"] != "表の題名" {
			t.Errorf("title = %q, want %q", tags["title"], "表の題名")
		}
		if tags["artist"] != "伊ヶ崎綾香" {
			t.Errorf("artist = %q, want %q", tags["artist"], "伊ヶ崎綾香")
		}
		if tags["genre"] != "音声作品" {
			t.Errorf("genre = %q, want %q", tags["genre"], "音声作品")
		}
		if tags["date"] != "2023" {
			t.Errorf("date = %q, want %q", tags["date"], "2023")
		}
		if tags["track"] != "1" {
			t.Errorf("track = %q, want %q", tags["track"], "1")
		}
		if tags["comment"] != "あ\\\n" {
			t.Errorf("comment = %q, want %q", tags["comment"], "あ\\\n")
		}
	})
}

// TestFFmpegKeepsTheFirstTagValue pins that the repair changes encoding only, not
// which of several same-named values a container wins with. ffmpeg copies the
// first match, so a file whose id3 chunk comes first must keep reporting its id3
// text even when a later legacy INFO chunk disagrees — repairing the legacy
// bytes there would replace a correct tag with an older one.
func TestFFmpegKeepsTheFirstTagValue(t *testing.T) {
	cases := []struct {
		name     string
		info     []riffInfo
		frames   []id3Frame
		id3First bool
		want     string
	}{
		{
			name:   "legacy INFO first is decoded, even though id3 disagrees",
			info:   []riffInfo{{id: "INAM", value: shiftJIS(t, "古い題名")}},
			frames: []id3Frame{{id: "TIT2", text: "新しい題名"}},
			want:   "古い題名",
		},
		{
			name:     "id3 first keeps its Shift-JIS-disagreeing INFO from winning",
			info:     []riffInfo{{id: "INAM", value: shiftJIS(t, "古い題名")}},
			frames:   []id3Frame{{id: "TIT2", text: "新しい題名"}},
			id3First: true,
			want:     "新しい題名",
		},
		{
			name:     "id3 first keeps its ASCII INFO from winning",
			info:     []riffInfo{{id: "INAM", value: []byte("Old Title")}},
			frames:   []id3Frame{{id: "TIT2", text: "New Title"}},
			id3First: true,
			want:     "New Title",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoder := tagEncoder(t)
			src := filepath.Join(t.TempDir(), "values.wav")
			writeTaggedWAV(t, src, tc.info, tc.frames, tc.id3First)
			encodeAllTargets(t, encoder, src, func(t *testing.T, _ reconcile.Codec, tags map[string]string) {
				if tags["title"] != tc.want {
					t.Errorf("title = %q, want %q", tags["title"], tc.want)
				}
			})
		})
	}
}

// TestFFmpegRepairsTheValueTheContainerSelects pins which value of a repeated
// key the repair acts on, for the two ways a WAV repeats one.
//
// Inside a single INFO chunk ffmpeg's own dictionary has already collapsed the
// key to its last value before ffprobe reports it, so the surviving value is the
// one to decode. Across an INFO chunk and an id3 chunk both values survive and
// ffmpeg copies the first — so the undecodable first value must stay the output's
// tag rather than the id3 text behind it taking its place.
func TestFFmpegRepairsTheValueTheContainerSelects(t *testing.T) {
	t.Run("repeated inside the INFO chunk: the surviving last value", func(t *testing.T) {
		encoder := tagEncoder(t)
		src := filepath.Join(t.TempDir(), "repeated.wav")
		writeTaggedWAV(t, src, []riffInfo{
			{id: "INAM", value: shiftJIS(t, "古い題名")},
			{id: "INAM", value: shiftJIS(t, "新しい題名")},
		}, nil, false)
		encodeAllTargets(t, encoder, src, func(t *testing.T, _ reconcile.Codec, tags map[string]string) {
			if tags["title"] != "新しい題名" {
				t.Errorf("title = %q, want %q", tags["title"], "新しい題名")
			}
		})
	})

	t.Run("across INFO and id3: the undecodable first value keeps its place", func(t *testing.T) {
		encoder := tagEncoder(t)
		src := filepath.Join(t.TempDir(), "locked.wav")
		writeTaggedWAV(t, src, []riffInfo{
			{id: "INAM", value: []byte{0x80, 0x80}}, // decodable in neither UTF-8 nor Shift-JIS
		}, []id3Frame{{id: "TIT2", text: "新しい題名"}}, false)
		encodeAllTargets(t, encoder, src, func(t *testing.T, _ reconcile.Codec, tags map[string]string) {
			if tags["title"] == "新しい題名" {
				t.Errorf("the id3 value displaced the first: title = %q", tags["title"])
			}
		})
	})
}

// TestFFmpegConvertsSourcesWithUnusableTags pins that tags nothing can turn into
// text never fail a conversion: the source is media to convert, and its metadata
// is copied as faithfully as it can be rather than gating the encode.
func TestFFmpegConvertsSourcesWithUnusableTags(t *testing.T) {
	cases := []struct {
		name string
		info []riffInfo
	}{
		{name: "undecodable bytes", info: []riffInfo{{id: "INAM", value: []byte{0x80, 0xFF}}}},
		{name: "embedded NUL", info: []riffInfo{{id: "INAM", value: []byte{0x82, 0xA0, 0x00, 0x82, 0xA0}}}},
		{name: "empty value", info: []riffInfo{{id: "INAM", value: nil}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoder := tagEncoder(t)
			src := filepath.Join(t.TempDir(), "unusable.wav")
			writeTaggedWAV(t, src, tc.info, nil, false)
			dst := filepath.Join(t.TempDir(), "output.flac")
			err := encoder.Encode(t.Context(), src, dst,
				reconcile.AudioOutputSpec{Codec: reconcile.CodecFlac})
			if err != nil {
				t.Fatalf("tags gated the conversion: %v", err)
			}
		})
	}
}
