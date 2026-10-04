package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/settings"
)

// TestPlanReadsKeysOverTheirDefaults pins what an omitted key means. The
// configuration is optional in both directions: a file that is absent entirely
// answers the compiled-in defaults, and one that is present but silent about a
// key keeps that key's default too — an omitted batch_update is the batched
// write shape, not the per-row one, so adding an unrelated key to config.json
// can never change how probed rates are persisted.
func TestPlanReadsKeysOverTheirDefaults(t *testing.T) {
	for _, tc := range []struct {
		name        string
		config      string // empty writes no file at all
		wantBatch   bool
		wantFFprobe string
	}{
		{name: "no file at all", config: "", wantBatch: true},
		{name: "empty object", config: `{}`, wantBatch: true},
		{name: "only a probe path", config: `{"tools":{"ffprobe_path":"/opt/ffprobe"}}`, wantBatch: true, wantFFprobe: "/opt/ffprobe"},
		{name: "explicit false", config: `{"plan":{"bitrate":{"batch_update":false}}}`, wantBatch: false},
		{name: "explicit true", config: `{"plan":{"bitrate":{"batch_update":true}}}`, wantBatch: true},
		{name: "unparsable", config: `{not json`, wantBatch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.config != "" {
				if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(tc.config), 0o644); err != nil {
					t.Fatalf("write config: %v", err)
				}
			}
			got := settings.NewReader(dir).Plan()
			if got.BatchUpdate != tc.wantBatch {
				t.Fatalf("BatchUpdate = %v, want %v", got.BatchUpdate, tc.wantBatch)
			}
			if got.FFprobePath != tc.wantFFprobe {
				t.Fatalf("FFprobePath = %q, want %q", got.FFprobePath, tc.wantFFprobe)
			}
		})
	}
}

// TestToolsFallsBackToPath covers the read the execution path prepares with:
// only what the file names is used, and nothing named means PATH.
func TestToolsFallsBackToPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"),
		[]byte(`{"tools":{"ffmpeg_path":"/opt/ffmpeg"}}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	tools := settings.NewReader(dir).Tools()
	if tools.FFmpegPath != "/opt/ffmpeg" || tools.FFprobePath != "" {
		t.Fatalf("tools = %+v, want the configured encoder and an empty probe", tools)
	}
	if empty := (settings.NewReader("").Tools()); empty.FFmpegPath != "" || empty.FFprobePath != "" {
		t.Fatalf("tools without a configuration = %+v, want the zero value", empty)
	}
}

func TestScanWorkersDefaultsAndBounds(t *testing.T) {
	for _, tc := range []struct {
		config string
		want   int
	}{
		{`{}`, 32}, {`{"scan":{"workers":1}}`, 1}, {`{"scan":{"workers":16}}`, 16},
		{`{"scan":{"workers":32}}`, 32}, {`{"scan":{"workers":33}}`, 32},
		{`{"scan":{"workers":0}}`, 32}, {`invalid`, 32},
	} {
		t.Run(tc.config, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(tc.config), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := settings.NewReader(dir).ScanWorkers(); got != tc.want {
				t.Fatalf("workers = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEncodeWorkersDefaultsAndBounds(t *testing.T) {
	for _, tc := range []struct {
		config string
		want   int
	}{
		{`{}`, 0}, {`{"execute":{"max_io_workers":1}}`, 1}, {`{"execute":{"max_io_workers":4}}`, 4},
		{`{"execute":{"max_io_workers":6}}`, 6}, {`{"execute":{"max_io_workers":32}}`, 32},
		{`{"execute":{"max_io_workers":33}}`, 0}, {`{"execute":{"max_io_workers":0}}`, 0},
		{`{"execute":{"max_io_workers":-1}}`, 0}, {`{"execute":{"max_io_workers":"four"}}`, 0}, {`invalid`, 0},
	} {
		t.Run(tc.config, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(tc.config), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := settings.NewReader(dir).EncodeWorkers(); got != tc.want {
				t.Fatalf("workers=%d want %d", got, tc.want)
			}
		})
	}
	if got := settings.NewReader(t.TempDir()).EncodeWorkers(); got != 0 {
		t.Fatalf("missing configuration: %d", got)
	}
}
