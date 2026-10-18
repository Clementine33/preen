package filesystem_test

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
)

func TestTruncatePathComponentsToBytes_UTF8Boundary(t *testing.T) {
	longComponent := strings.Repeat("单", 120) // 360 bytes in UTF-8
	if len(longComponent) <= 214 {
		t.Fatalf("test setup invalid: component bytes=%d, need >214", len(longComponent))
	}

	input := filepath.Join("1-单一", "12_一般", longComponent)
	got := filesystem.TruncatePathComponentsToBytes(input, 214)

	parts := strings.SplitSeq(got, string(filepath.Separator))
	for part := range parts {
		if part == "" || part == "." || part == ".." {
			continue
		}
		if len(part) > 214 {
			t.Fatalf("component bytes=%d exceeds 214: %q", len(part), part)
		}
		if !utf8.ValidString(part) {
			t.Fatalf("component is not valid UTF-8: %q", part)
		}
	}
}
