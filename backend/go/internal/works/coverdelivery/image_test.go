package coverdelivery_test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/jpeg"
	"testing"

	"github.com/onsei/organizer/backend/internal/works/coverdelivery"
)

func TestInspectUsesActualImageFormat(t *testing.T) {
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
		t.Fatal(err)
	}
	webp, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		data []byte
		ext  string
	}{{jpg.Bytes(), "jpg"}, {webp, "webp"}} {
		ext, _, err := coverdelivery.Inspect(tc.data)
		if err != nil || ext != tc.ext {
			t.Fatalf("format: %s %v", ext, err)
		}
	}
}
func TestInspectRejectsDamagedWebPAndNonImage(t *testing.T) {
	raw := make([]byte, 26)
	copy(raw, "RIFF")
	binary.LittleEndian.PutUint32(raw[4:8], 18)
	copy(raw[8:], "WEBPVP8L")
	binary.LittleEndian.PutUint32(raw[16:20], 5)
	raw[20] = 0x2f
	for _, data := range [][]byte{raw, []byte("<html>denied</html>")} {
		if _, _, err := coverdelivery.Inspect(data); coverdelivery.Code(err) != "INVALID_IMAGE" {
			t.Fatalf("invalid image accepted: %v", err)
		}
	}
}
