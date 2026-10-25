package ffmpeg_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/ffmpeg"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
)

func oggPageStarts(data []byte) []int {
	var starts []int
	for pos := 0; pos < len(data); {
		starts = append(starts, pos)
		segments := int(data[pos+26])
		length := 0
		for _, size := range data[pos+27 : pos+27+segments] {
			length += int(size)
		}
		pos += 27 + segments + length
	}
	return starts
}

// An independent bit-at-a-time CRC allows structural mutations with valid CRC.
func repairOggCRC(data []byte, start int) {
	segments := int(data[start+26])
	length := 0
	for _, size := range data[start+27 : start+27+segments] {
		length += int(size)
	}
	page := data[start : start+27+segments+length]
	clear(page[22:26])
	var crc uint32
	for _, b := range page {
		crc ^= uint32(b) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04c11db7
			} else {
				crc <<= 1
			}
		}
	}
	binary.LittleEndian.PutUint32(page[22:], crc)
}

func TestOpusPageIntegrity(t *testing.T) {
	_, src := audioFixture(t)
	spec := reconcile.AudioOutputSpec{
		Codec:   reconcile.CodecOpus,
		Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 128},
	}
	encoder, source, original := outputFixture(t, src, spec)
	pages := oggPageStarts(original)
	last := pages[len(pages)-1]
	middle := pages[2]
	faults := map[string]func([]byte) []byte{
		"truncated header":           func(b []byte) []byte { return b[:last+10] },
		"truncated payload":          func(b []byte) []byte { return b[:len(b)-8] },
		"CRC":                        func(b []byte) []byte { b[len(b)-1] ^= 1; return b },
		"missing EOS":                func(b []byte) []byte { b[last+5] &^= 4; repairOggCRC(b, last); return b },
		"missing page":               func(b []byte) []byte { return append(b[:middle], b[pages[3]:]...) },
		"sequence":                   func(b []byte) []byte { b[middle+18]++; repairOggCRC(b, middle); return b },
		"continued":                  func(b []byte) []byte { b[middle+5] ^= 1; repairOggCRC(b, middle); return b },
		"serial":                     func(b []byte) []byte { b[middle+14] ^= 1; repairOggCRC(b, middle); return b },
		"BOS":                        func(b []byte) []byte { b[middle+5] |= 2; repairOggCRC(b, middle); return b },
		"extra whole page after EOS": func(b []byte) []byte { return append(b, b[last:]...) },
	}
	for name, mutate := range faults {
		t.Run(name, func(t *testing.T) {
			data := mutate(bytes.Clone(original))
			validateBytes(t, encoder, source, spec, data, false)
			path := filepath.Join(t.TempDir(), "damaged.opus")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			_, err = ffmpeg.HashOpusForTest(t.Context(), file)
			if err == nil {
				t.Fatal("native validator accepted corrupt stream")
			}
			if strings.Contains(name, "after EOS") && !strings.Contains(err.Error(), "trailing bytes after EOS") {
				t.Fatalf("diagnostic: %v", err)
			}
		})
	}
}

func TestEncodingDropsChapters(t *testing.T) {
	_, wav := audioFixture(t)
	dir := t.TempDir()
	metadata := filepath.Join(dir, "chapters.txt")
	if err := os.WriteFile(
		metadata,
		[]byte(";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1000\ntitle=Opening\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(dir, "source.m4a")
	runFFmpeg(
		t,
		"-i",
		wav,
		"-f",
		"ffmetadata",
		"-i",
		metadata,
		"-map_metadata",
		"1",
		"-c:a",
		"flac",
		"-strict",
		"experimental",
		"-f",
		"mp4",
		sourcePath,
	)
	wavData, err := os.ReadFile(wav)
	if err != nil {
		t.Fatal(err)
	}
	const cueCount = 198
	cues := uint32le(cueCount)
	for i := range cueCount {
		cue := append(uint32le(i+1), make([]byte, 20)...)
		copy(cue[8:], "data") // DAW markers at the same position, without labels.
		cues = append(cues, cue...)
	}
	wavData = append(wavData, riffChunk("cue ", cues)...)
	copy(wavData[4:8], uint32le(len(wavData)-8))
	cuePath := filepath.Join(dir, "cue.wav")
	if err := os.WriteFile(cuePath, wavData, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, source := range []struct {
		name     string
		path     string
		chapters int
	}{
		{"titled M4A", sourcePath, 1},
		{"untitled WAV cues", cuePath, cueCount},
	} {
		t.Run(source.name, func(t *testing.T) {
			assertChapterCount(t, source.path, source.chapters)
			for _, codec := range []reconcile.Codec{
				reconcile.CodecWav, reconcile.CodecFlac, reconcile.CodecMp3, reconcile.CodecAac, reconcile.CodecOpus,
			} {
				t.Run(string(codec), func(t *testing.T) {
					spec := reconcile.AudioOutputSpec{Codec: codec}
					if !spec.Lossless() {
						spec.Quality = &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 128}
					}
					_, _, data := outputFixture(t, source.path, spec)
					path := filepath.Join(t.TempDir(), "output"+reconcile.ExtForCodec(codec))
					if err := os.WriteFile(path, data, 0o600); err != nil {
						t.Fatal(err)
					}
					assertChapterCount(t, path, 0)
				})
			}
		})
	}
}

func assertChapterCount(t *testing.T, path string, want int) {
	t.Helper()
	data, err := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-show_chapters", "-of", "json", path).
		CombinedOutput()
	if err != nil {
		t.Fatalf("probe chapters: %v: %s", err, data)
	}
	var result struct {
		Chapters []json.RawMessage `json:"chapters"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("probe chapters: %v: %s", err, data)
	}
	if len(result.Chapters) != want {
		t.Fatalf("chapters = %d, want %d", len(result.Chapters), want)
	}
}

func TestOpusContinuedTagPages(t *testing.T) {
	_, wav := audioFixture(t)
	dir := t.TempDir()
	metadata := filepath.Join(dir, "tags.txt")
	if err := os.WriteFile(
		metadata,
		[]byte(";FFMETADATA1\ntitle="+strings.Repeat("long metadata ", 20000)+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "tagged.flac")
	runFFmpeg(t, "-i", wav, "-f", "ffmetadata", "-i", metadata, "-map_metadata", "1", "-c:a", "flac", source)
	spec := reconcile.AudioOutputSpec{
		Codec:   reconcile.CodecOpus,
		Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 128},
	}
	_, _, data := outputFixture(t, source, spec)
	continued := false
	for _, pos := range oggPageStarts(data) {
		continued = continued || data[pos+5]&1 != 0
	}
	if !continued {
		t.Fatal("fixture did not create a continued tags packet")
	}
}
