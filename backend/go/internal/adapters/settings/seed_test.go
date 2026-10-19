package settings_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/settings"
)

func TestSeedDefaultsMakesInitialTagsReadable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	reader := settings.NewReader(dir)
	if len(reader.PruneNoEffectTags()) != 0 || len(reader.PruneOtherTags()) != 0 {
		t.Fatal("missing file must not use embedded tag defaults")
	}
	if err := settings.SeedDefaults(dir); err != nil {
		t.Fatal(err)
	}
	want := []string{"nose", "secut", "seなし", "se無し", "无se", "无效果音", "无音效", "無se", "無效果音", "無音效", "音cut", "音なし", "音無し"}
	if !slices.Equal(reader.PruneNoEffectTags(), want) || !slices.Equal(reader.PruneOtherTags(), []string{"反転", "反转"}) {
		t.Fatalf("initial tags = %v / %v", reader.PruneNoEffectTags(), reader.PruneOtherTags())
	}
	if err := os.Remove(filepath.Join(dir, "config.json")); err != nil {
		t.Fatal(err)
	}
	if len(reader.PruneNoEffectTags()) != 0 || len(reader.PruneOtherTags()) != 0 {
		t.Fatal("deleted file must not use embedded tag defaults")
	}
}

func TestSeedDefaultsPreservesExistingFiles(t *testing.T) {
	for _, tc := range []struct {
		contents string
		invalid  bool
	}{
		{`{"prune":{"no_effect_tags":["custom"]}}`, false},
		{"{broken\n", true},
		{"", true},
	} {
		t.Run(tc.contents, func(t *testing.T) {
			contents := tc.contents
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, []byte(contents), 0o400); err != nil {
				t.Fatal(err)
			}
			if err := settings.SeedDefaults(dir); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != contents {
				t.Fatalf("existing configuration changed: %q, %v", got, err)
			}
			if _, err := settings.NewReader(dir).Server("", nil, nil); (err != nil) != tc.invalid {
				t.Fatalf("existing server validation changed: %v", err)
			}
		})
	}
}

func TestSeedDefaultsReportsInaccessibleDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(dir, []byte("user data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := settings.SeedDefaults(dir); err == nil {
		t.Fatal("configuration delivery failure must fail startup")
	}
	got, err := os.ReadFile(dir)
	if err != nil || string(got) != "user data" {
		t.Fatalf("inaccessible directory changed: %q, %v", got, err)
	}
}

func TestSeedDefaultsConcurrentProcesses(t *testing.T) {
	if dir := os.Getenv("PREEN_SEED_TEST_DIR"); dir != "" {
		want, err := os.ReadFile("config.json.template")
		if err != nil {
			t.Fatal(err)
		}
		for round := range 16 {
			target := filepath.Join(dir, string(rune('a'+round)))
			if err := settings.SeedDefaults(target); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(target, "config.json"))
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("concurrent startup read incomplete configuration: %q, %v", got, err)
			}
		}
		return
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var commands []*exec.Cmd
	var outputs []*bytes.Buffer
	for range 4 {
		cmd := exec.CommandContext(t.Context(), bin, "-test.run=^TestSeedDefaultsConcurrentProcesses$")
		cmd.Env = append(os.Environ(), "PREEN_SEED_TEST_DIR="+dir)
		output := new(bytes.Buffer)
		cmd.Stdout, cmd.Stderr = output, output
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, cmd)
		outputs = append(outputs, output)
	}
	for i, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Errorf("concurrent startup: %v\n%s", err, outputs[i])
		}
	}
}
