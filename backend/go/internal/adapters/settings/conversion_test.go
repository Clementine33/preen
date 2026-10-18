package settings_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/settings"
)

// TestPlanReadsKeysOverTheirDefaults pins what an omitted key means. The
// configuration is optional in both directions: a file that is absent entirely
// answers the compiled-in defaults, and one that is present but silent about a
// key keeps that key's default too — an omitted batch_update is the batched
// write shape, not the per-row one, so adding an unrelated key to config.json
// can never change how probed rates are persisted.
func TestPlanReadsKeysOverTheirDefaults(t *testing.T) {
	for _, tc := range []struct {
		name      string
		config    string // empty writes no file at all
		wantBatch bool
	}{
		{name: "no file at all", config: "", wantBatch: true},
		{name: "empty object", config: `{}`, wantBatch: true},
		{name: "retired probe path", config: `{"tools":{"ffprobe_path":"/opt/ffprobe"}}`, wantBatch: true},
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
		})
	}
}

func TestRetiredToolsDoNotAffectTagSettings(t *testing.T) {
	dir := t.TempDir()
	config := `{"tools":{"ffmpeg_path":false,"ffprobe_path":42},"prune":{"no_effect_tags":["NoSE"]}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if tags := settings.NewReader(dir).PruneNoEffectTags(); !slices.Equal(tags, []string{"NoSE"}) {
		t.Fatalf("retired tool keys interfered with tags: %v", tags)
	}
}

func TestRetiredConcurrencyAndToolKeysDoNotAffectRemainingSettings(t *testing.T) {
	dir := t.TempDir()
	config := `{
		"scan": {"workers": "retired"},
		"execute": {"max_io_workers": "retired", "precheck": {"concurrent_stat": "retired"}},
		"workset": {"generation_concurrency": "retired"},
		"tools": {"ffmpeg_path": "retired", "ffprobe_path": "retired"},
		"plan": {"bitrate": {"batch_update": false}}
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	reader := settings.NewReader(dir)
	if plan := reader.Plan(); plan.BatchUpdate {
		t.Fatalf("retired keys interfered with planning configuration: %+v", plan)
	}
}

func TestConversionTagConfigSupportsIndependentGroupsAndExplicitEmpty(t *testing.T) {
	for _, tc := range []struct {
		config                  string
		wantNoEffect, wantOther []string
	}{
		{`{"prune":{"no_effect_tags":["SEなし"]}}`, []string{"SEなし"}, nil},
		{`{"prune":{"no_effect_tags":[],"other_tags":["反転"]}}`, nil, []string{"反転"}},
		{`{"prune":{"no_effect_tags":["NoSE"],"other_tags":[]}}`, []string{"NoSE"}, nil},
	} {
		t.Run(tc.config, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(tc.config), 0o644); err != nil {
				t.Fatal(err)
			}
			reader := settings.NewReader(dir)
			if !slices.Equal(reader.PruneNoEffectTags(), tc.wantNoEffect) ||
				!slices.Equal(reader.PruneOtherTags(), tc.wantOther) {
				t.Fatalf("tags = %v / %v", reader.PruneNoEffectTags(), reader.PruneOtherTags())
			}
		})
	}
}
