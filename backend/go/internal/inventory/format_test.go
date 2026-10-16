package inventory //nolint:testpackage // pins the unexported format rule

import "testing"

func TestDetectFormatGivesSidecarsNoMediaType(t *testing.T) {
	for path, want := range map[string]string{
		"/lib/RJ1/Track1.mp3":      "audio/mpeg",
		"/lib/RJ1/._Track1.mp3":    "",
		"/lib/RJ1/art/._cover.jpg": "",
		"/lib/RJ1/art/cover.jpg":   "image/jpeg",
	} {
		if got := detectFormatFromPath(path, false); got != want {
			t.Errorf("detectFormatFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}
