package media_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/media"
	"github.com/Clementine33/preen/backend/internal/thumbnail"
)

func writeImage(t *testing.T, name string, encode func(*bytes.Buffer) error) string {
	t.Helper()
	var buf bytes.Buffer
	if err := encode(&buf); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func decodedSize(t *testing.T, img thumbnail.Image) image.Point {
	t.Helper()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(img.Bytes))
	if err != nil {
		t.Fatal(err)
	}
	return image.Pt(cfg.Width, cfg.Height)
}

func TestThumbnailScalesOpaqueToJPEGAndKeepsAlphaAsPNG(t *testing.T) {
	opaque := image.NewRGBA(image.Rect(0, 0, 800, 400))
	for i := range opaque.Pix {
		opaque.Pix[i] = 0xff
	}
	jpegPath := writeImage(t, "cg.jpg", func(b *bytes.Buffer) error { return jpeg.Encode(b, opaque, nil) })
	img, err := media.ThumbEngine{}.Thumbnail(jpegPath, 120)
	if err != nil {
		t.Fatal(err)
	}
	if img.ContentType != "image/jpeg" || decodedSize(t, img) != image.Pt(240, 120) {
		t.Errorf("opaque thumbnail = %s %v, want image/jpeg 240x120", img.ContentType, decodedSize(t, img))
	}

	translucent := image.NewNRGBA(image.Rect(0, 0, 60, 90))
	translucent.Set(0, 0, color.NRGBA{A: 0x80})
	pngPath := writeImage(t, "logo.png", func(b *bytes.Buffer) error { return png.Encode(b, translucent) })
	img, err = media.ThumbEngine{}.Thumbnail(pngPath, 240)
	if err != nil {
		t.Fatal(err)
	}
	// Alpha survives as PNG, and a small source is never upscaled.
	if img.ContentType != "image/png" || decodedSize(t, img) != image.Pt(60, 90) {
		t.Errorf("alpha thumbnail = %s %v, want image/png 60x90", img.ContentType, decodedSize(t, img))
	}
}

func TestThumbnailRefusesBombsAndNoise(t *testing.T) {
	// A header claiming 20000x20000 over a 1x1 body: refused from the header.
	bomb := writeImage(t, "bomb.png", func(b *bytes.Buffer) error {
		if err := png.Encode(b, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
			return err
		}
		ihdr := b.Bytes()[8:]
		binary.BigEndian.PutUint32(ihdr[8:], 20000)
		binary.BigEndian.PutUint32(ihdr[12:], 20000)
		binary.BigEndian.PutUint32(ihdr[21:], crc32.ChecksumIEEE(ihdr[4:21]))
		return nil
	})
	if _, err := (media.ThumbEngine{}).Thumbnail(bomb, 120); !errors.Is(err, thumbnail.ErrTooLarge) {
		t.Errorf("bomb: got %v, want ErrTooLarge", err)
	}

	noise := writeImage(t, "noise.jpg", func(b *bytes.Buffer) error {
		_, err := b.WriteString("not an image")
		return err
	})
	if _, err := (media.ThumbEngine{}).Thumbnail(noise, 120); !errors.Is(err, thumbnail.ErrUndecodable) {
		t.Errorf("noise: got %v, want ErrUndecodable", err)
	}
}
