package covers

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"

	_ "golang.org/x/image/webp"
)

// Inspect validates image content; bytes are never re-encoded.
func Inspect(data []byte) (string, string, error) {
	bad := func() (string, string, error) {
		return "", "", Failure("INVALID_IMAGE", fmt.Errorf("invalid or unsupported image"))
	}
	if len(data) == 0 || len(data) > MaxBytes {
		return bad()
	}
	mime := http.DetectContentType(data)
	if mime == "image/webp" {
		if len(data) < 20 || int64(binary.LittleEndian.Uint32(data[4:8]))+8 != int64(len(data)) {
			return bad()
		}
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40000000 {
		return bad()
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return bad()
	}
	switch format {
	case "webp":
		return "webp", "image/webp", nil
	case "jpeg":
		return "jpg", "image/jpeg", nil
	case "png":
		return "png", "image/png", nil
	case "gif":
		return "gif", "image/gif", nil
	}
	return bad()
}
