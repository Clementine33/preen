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

func TestAudioDurationSecProbesRealFile(t *testing.T) {
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

	sec, err := media.New("").AudioDurationSec(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if sec < 1.9 || sec > 2.1 {
		t.Errorf("expected ~2s, got %v", sec)
	}
}

func TestAudioDurationSecRejectsNonAudio(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "noise.bin"))
	if err := os.WriteFile(filepath.FromSlash(path), []byte("not audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := media.New("").AudioDurationSec(context.Background(), path); err == nil {
		t.Fatal("expected an error for a non-audio file")
	}
}
