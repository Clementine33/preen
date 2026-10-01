package media_test

import (
	"context"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/media"
)

func TestImageDimensionsReadsHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "cover.png"))
	f, err := os.Create(filepath.FromSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	if encodeErr := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 1200, 900))); encodeErr != nil {
		t.Fatal(encodeErr)
	}
	f.Close()

	width, height, err := media.New("").ImageDimensions(path)
	if err != nil {
		t.Fatal(err)
	}
	if width != 1200 || height != 900 {
		t.Errorf("expected 1200x900, got %dx%d", width, height)
	}
}

func TestImageDimensionsRejectsNonImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "noise.bin"))
	if err := os.WriteFile(filepath.FromSlash(path), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := media.New("").ImageDimensions(path); err == nil {
		t.Fatal("expected an error for a non-image file")
	}
}

func TestAudioFactsProbesRealFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "track.wav"))
	output, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i",
		"sine=frequency=440:duration=2", path).CombinedOutput()
	if err != nil {
		t.Fatalf("generate wav: %v: %s", err, output)
	}

	sec, _, err := media.New("").AudioFacts(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if sec < 1.9 || sec > 2.1 {
		t.Errorf("expected ~2s, got %v", sec)
	}
}

func TestAudioFactsRejectsNonAudio(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "noise.bin"))
	if err := os.WriteFile(filepath.FromSlash(path), []byte("not audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := media.New("").AudioFacts(context.Background(), path); err == nil {
		t.Fatal("expected an error for a non-audio file")
	}
}

func TestAudioFactsReadsLiveToolPathOnlyForFallback(t *testing.T) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	dir := t.TempDir()
	wav, flac := filepath.Join(dir, "tone.wav"), filepath.Join(dir, "tone.flac")
	for _, path := range []string{wav, flac} {
		out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi",
			"-i", "sine=duration=2", path).CombinedOutput()
		if err != nil {
			t.Fatalf("generate %s: %v: %s", path, err, out)
		}
	}
	p := media.New(filepath.Join(dir, "missing-at-startup"))
	p.PathSource = func() string { t.Error("native WAV read tool settings"); return "" }
	if sec, _, err := p.AudioFacts(context.Background(), wav); err != nil || sec != 2 {
		t.Fatalf("native probe = %v, %v", sec, err)
	}
	path := ffprobe
	p.PathSource = func() string { return path }
	if sec, _, err := p.AudioFacts(context.Background(), flac); err != nil || sec != 2 {
		t.Fatalf("configured fallback = %v, %v", sec, err)
	}
	path = filepath.Join(dir, "changed-to-missing")
	if _, _, err := p.AudioFacts(context.Background(), flac); err == nil {
		t.Error("fallback retained the old tool path after settings changed")
	}
}
