package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"

	"golang.org/x/image/draw"

	"github.com/Clementine33/preen/backend/internal/thumbnail"
)

// ThumbEngineVersion names the thumbnail engine's output. Bump it whenever the
// rendering changes: it is part of every thumbnail address (ADR 0013), so
// a bump retires every cached copy, the browsers' included.
const ThumbEngineVersion = "go-bilinear-1"

// ThumbEngine is the first thumbnail engine of ADR 0013: the standard
// decoders plus x/image's webp, x/image/draw's bilinear scaling, JPEG output
// for opaque sources and PNG for the rest. It runs in-process.
type ThumbEngine struct{}

// Thumbnail decodes path and scales it to height, never upscaling. The pixel
// count is checked from the header before any pixel is decoded.
func (ThumbEngine) Thumbnail(path string, height int) (thumbnail.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return thumbnail.Image{}, err
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return thumbnail.Image{}, fmt.Errorf("%w: %w", thumbnail.ErrUndecodable, err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > thumbnail.MaxPixels {
		return thumbnail.Image{}, thumbnail.ErrTooLarge
	}
	if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		return thumbnail.Image{}, seekErr
	}
	src, _, err := image.Decode(f)
	if err != nil {
		return thumbnail.Image{}, fmt.Errorf("%w: %w", thumbnail.ErrUndecodable, err)
	}

	bounds := src.Bounds()
	h := min(height, bounds.Dy())
	w := max(1, bounds.Dx()*h/max(1, bounds.Dy()))
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Src, nil)

	var out bytes.Buffer
	if opaque, ok := src.(interface{ Opaque() bool }); ok && opaque.Opaque() {
		err = jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85})
		return thumbnail.Image{Bytes: out.Bytes(), ContentType: "image/jpeg"}, err
	}
	err = png.Encode(&out, dst)
	return thumbnail.Image{Bytes: out.Bytes(), ContentType: "image/png"}, err
}
